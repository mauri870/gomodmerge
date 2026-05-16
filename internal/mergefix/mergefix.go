package mergefix

import (
	"bytes"
	"errors"
	"regexp"
	"sort"

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

// MergeGoMod resolves conflicts in go.mod by picking the semver-max version
// for conflicting require directives. Returns ErrorNoConflicts if no conflict
// markers are present, or ErrorUnsupportedDirective if replace/exclude directives
// appear inside a conflict block.
func MergeGoMod(data []byte) ([]byte, error) {
	if !hasConflictMarkers(data) {
		return nil, ErrorNoConflicts
	}

	state := stateNormal
	var out []byte
	var ours, theirs [][]byte

	for line := range bytes.Lines(data) {
		line = bytes.TrimRight(line, "\r\n")
		switch {
		case ParentMergeRE.Match(line):
			state = stateParent
		case OursMergeRE.Match(line):
			state = stateOurs
			ours = nil
		case TheirsMergeRE.Match(line):
			state = stateTheirs
			theirs = nil
		case EndMergeRE.Match(line):
			resolved, err := resolveGoModConflict(ours, theirs)
			if err != nil {
				return nil, err
			}
			for _, l := range resolved {
				out = append(out, l...)
				out = append(out, '\n')
			}
			state = stateNormal
			ours = nil
			theirs = nil
		default:
			switch state {
			case stateNormal:
				out = append(out, line...)
				out = append(out, '\n')
			case stateOurs:
				ours = append(ours, append([]byte(nil), line...))
			case stateTheirs:
				theirs = append(theirs, append([]byte(nil), line...))
			}
		}
	}

	return out, nil
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

type requireEntry struct {
	module  string
	version string
}

// parseRequireLine extracts the module path and version from a go.mod require line.
// It handles both the single-line form ("require module/path v1.2.3") and the
// block form ("\tmodule/path v1.2.3 // indirect").
func parseRequireLine(line []byte) (requireEntry, bool) {
	trimmed := bytes.TrimSpace(line)
	trimmed = bytes.TrimPrefix(trimmed, []byte("require "))
	fields := bytes.Fields(trimmed)
	if len(fields) < 2 {
		return requireEntry{}, false
	}
	// Skip structural block delimiters: '(' and ')'
	if bytes.Equal(fields[0], []byte("(")) || bytes.Equal(fields[0], []byte(")")) {
		return requireEntry{}, false
	}
	// Version must start with 'v' for a valid module version
	if !bytes.HasPrefix(fields[1], []byte("v")) {
		return requireEntry{}, false
	}
	return requireEntry{
		module:  string(fields[0]),
		version: string(fields[1]),
	}, true
}

func isUnsupportedDirective(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.HasPrefix(trimmed, []byte("replace")) || bytes.HasPrefix(trimmed, []byte("exclude"))
}

// isStructural reports whether a line is a go.mod block delimiter that carries
// no semantic meaning of its own (e.g. "require (", "(", ")").
func isStructural(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return bytes.Equal(trimmed, []byte("(")) ||
		bytes.Equal(trimmed, []byte(")")) ||
		bytes.Equal(trimmed, []byte("require ("))
}

// resolveGoModConflict merges two sets of go.mod lines from a conflict block.
// For require directives that name the same module, the semver-max version is
// selected. Non-require lines are kept from ours (HEAD). Structural block
// delimiters are dropped since go mod tidy will reformat the file.
func resolveGoModConflict(ours, theirs [][]byte) ([][]byte, error) {
	for _, line := range append(ours, theirs...) {
		if isUnsupportedDirective(line) {
			return nil, ErrorUnsupportedDirective
		}
	}

	// Collect require entries from both sides
	oursReqs := make(map[string]string)
	for _, line := range ours {
		if entry, ok := parseRequireLine(line); ok {
			oursReqs[entry.module] = entry.version
		}
	}
	theirsReqs := make(map[string]string)
	for _, line := range theirs {
		if entry, ok := parseRequireLine(line); ok {
			theirsReqs[entry.module] = entry.version
		}
	}

	// Build merged map: pick semver-max version per module
	merged := make(map[string]string)
	for mod, ver := range oursReqs {
		merged[mod] = ver
	}
	for mod, ver := range theirsReqs {
		if existing, ok := merged[mod]; ok {
			if compareVersions(ver, existing) > 0 {
				merged[mod] = ver
			}
		} else {
			merged[mod] = ver
		}
	}

	// Emit resolved lines in ours order, then theirs-only additions (sorted for determinism)
	var result [][]byte
	seen := make(map[string]bool)

	for _, line := range ours {
		entry, isReq := parseRequireLine(line)
		if isReq {
			if !seen[entry.module] {
				seen[entry.module] = true
				result = append(result, replaceVersion(line, entry.module, entry.version, merged[entry.module]))
			}
		} else if !isStructural(line) {
			result = append(result, line)
		}
	}

	var theirsOnly []string
	for mod := range merged {
		if !seen[mod] {
			theirsOnly = append(theirsOnly, mod)
		}
	}
	sort.Strings(theirsOnly)
	for _, mod := range theirsOnly {
		result = append(result, []byte("require "+mod+" "+merged[mod]))
	}

	return result, nil
}

// replaceVersion replaces oldVersion with newVersion in line, searching only
// after the module path to avoid corrupting module paths that contain the
// version string (e.g. example.com/v1.2.3/pkg at version v1.2.3).
func replaceVersion(line []byte, module, oldVersion, newVersion string) []byte {
	if oldVersion == newVersion {
		return line
	}
	modIdx := bytes.Index(line, []byte(module))
	if modIdx == -1 {
		return line
	}
	after := modIdx + len(module)
	verIdx := bytes.Index(line[after:], []byte(oldVersion))
	if verIdx == -1 {
		return line
	}
	pos := after + verIdx
	result := make([]byte, 0, len(line)-len(oldVersion)+len(newVersion))
	result = append(result, line[:pos]...)
	result = append(result, []byte(newVersion)...)
	result = append(result, line[pos+len(oldVersion):]...)
	return result
}

func compareVersions(a, b string) int {
	return semver.Compare(a, b)
}
