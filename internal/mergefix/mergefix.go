package mergefix

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

var (
	ParentMergeRE = regexp.MustCompile(`\|{7,}`)
	OursMergeRE   = regexp.MustCompile(`\<{7,}`)
	TheirsMergeRE = regexp.MustCompile(`\={7,}`)
	EndMergeRE    = regexp.MustCompile(`\>{7,}`)

	ErrorUnsupportedDirective = errors.New("unsupported directive. Please fix the conflicts manually.")
	ErrorNoConflicts          = errors.New("no conflicts to be fixed")
)

const (
	stateNormal = iota
	stateParent
	stateOurs
	stateTheirs
)

func hasConflictMarkers(b []byte) bool {
	return OursMergeRE.Match(b) && TheirsMergeRE.Match(b) && EndMergeRE.Match(b)
}

func isUnsupportedDirective(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.HasPrefix(trimmed, []byte("replace")) || bytes.HasPrefix(trimmed, []byte("exclude"))
}

// MergeGoMod resolves conflicts in go.mod by constructing two complete go.mod
// files from the ours and theirs sides of every conflict block, parsing both
// with modfile, and merging semantically: semver-max for require directives,
// higher version for go and toolchain directives.
// Returns ErrorNoConflicts if no conflict markers are present, or
// ErrorUnsupportedDirective if a replace/exclude appears inside a conflict block.
func MergeGoMod(data []byte) ([]byte, error) {
	if !hasConflictMarkers(data) {
		return nil, ErrorNoConflicts
	}

	oursData, theirsData, err := splitIntoSides(data)
	if err != nil {
		return nil, err
	}

	ours, err := modfile.Parse("go.mod", oursData, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing ours: %w", err)
	}
	theirs, err := modfile.Parse("go.mod", theirsData, nil)
	if err != nil {
		return nil, fmt.Errorf("parsing theirs: %w", err)
	}

	if err := mergeModFiles(ours, theirs); err != nil {
		return nil, err
	}

	return modfile.Format(ours.Syntax), nil
}

// splitIntoSides builds two complete go.mod byte slices from a conflicted file:
// one with the ours side substituted for every conflict block, one with theirs.
// Non-conflicting lines appear in both. Returns ErrorUnsupportedDirective if a
// replace or exclude directive appears inside a conflict block.
func splitIntoSides(data []byte) (oursData, theirsData []byte, err error) {
	state := stateNormal
	for line := range bytes.Lines(data) {
		line = bytes.TrimRight(line, "\r\n")
		switch {
		case ParentMergeRE.Match(line):
			state = stateParent
		case OursMergeRE.Match(line):
			state = stateOurs
		case TheirsMergeRE.Match(line):
			state = stateTheirs
		case EndMergeRE.Match(line):
			state = stateNormal
		default:
			if (state == stateOurs || state == stateTheirs) && isUnsupportedDirective(line) {
				return nil, nil, ErrorUnsupportedDirective
			}
			switch state {
			case stateNormal:
				oursData = appendLine(oursData, line)
				theirsData = appendLine(theirsData, line)
			case stateOurs:
				oursData = appendLine(oursData, line)
			case stateTheirs:
				theirsData = appendLine(theirsData, line)
			// stateParent: skip common ancestor lines
			}
		}
	}
	return oursData, theirsData, nil
}

func appendLine(buf, line []byte) []byte {
	return append(append(buf, line...), '\n')
}

// mergeModFiles applies theirs onto ours in place:
//   - require: semver-max per module; theirs-only modules are added
//   - go directive: higher version wins
//   - toolchain: higher version wins
func mergeModFiles(ours, theirs *modfile.File) error {
	theirsReqs := make(map[string]string)
	for _, r := range theirs.Require {
		theirsReqs[r.Mod.Path] = r.Mod.Version
	}

	for _, r := range ours.Require {
		if theirsVer, ok := theirsReqs[r.Mod.Path]; ok {
			if semver.Compare(theirsVer, r.Mod.Version) > 0 {
				if err := ours.AddRequire(r.Mod.Path, theirsVer); err != nil {
					return err
				}
			}
			delete(theirsReqs, r.Mod.Path)
		}
	}

	paths := make([]string, 0, len(theirsReqs))
	for path := range theirsReqs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := ours.AddRequire(path, theirsReqs[path]); err != nil {
			return err
		}
	}

	if theirs.Go != nil {
		if ours.Go == nil || semver.Compare("v"+theirs.Go.Version, "v"+ours.Go.Version) > 0 {
			if err := ours.AddGoStmt(theirs.Go.Version); err != nil {
				return err
			}
		}
	}

	if theirs.Toolchain != nil {
		if ours.Toolchain == nil || semver.Compare(
			"v"+theirs.Toolchain.Name[len("go"):],
			"v"+ours.Toolchain.Name[len("go"):],
		) > 0 {
			if err := ours.AddToolchainStmt(theirs.Toolchain.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// MergeGoSum resolves conflicts in go.sum by union-merging both sides of every
// conflict block, deduplicating, and sorting. Returns ErrorNoConflicts if no
// conflict markers are present.
func MergeGoSum(data []byte) ([]byte, error) {
	if !hasConflictMarkers(data) {
		return nil, ErrorNoConflicts
	}

	state := stateNormal
	seen := make(map[string]bool)
	var lines [][]byte

	addLine := func(line []byte) {
		if len(bytes.TrimSpace(line)) == 0 {
			return
		}
		key := string(line)
		if !seen[key] {
			seen[key] = true
			lines = append(lines, append([]byte(nil), line...))
		}
	}

	for line := range bytes.Lines(data) {
		line = bytes.TrimRight(line, "\r\n")
		switch {
		case ParentMergeRE.Match(line):
			state = stateParent
		case OursMergeRE.Match(line):
			state = stateOurs
		case TheirsMergeRE.Match(line):
			state = stateTheirs
		case EndMergeRE.Match(line):
			state = stateNormal
		default:
			if state != stateParent {
				addLine(line)
			}
		}
	}

	sort.Slice(lines, func(i, j int) bool {
		return bytes.Compare(lines[i], lines[j]) < 0
	})

	var result []byte
	for _, line := range lines {
		result = append(result, line...)
		result = append(result, '\n')
	}
	return result, nil
}
