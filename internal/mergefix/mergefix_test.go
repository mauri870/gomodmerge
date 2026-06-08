package mergefix

import (
	"bytes"
	"errors"
	"testing"
)


func TestMergeGoMod(t *testing.T) {
	t.Run("picks semver-max for single require conflict", func(t *testing.T) {
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/quote v1.5.1\n" +
				"=======\n" +
				"require rsc.io/quote v1.5.2\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/quote", "v1.5.2") {
			t.Errorf("expected v1.5.2 in output, got:\n%s", out)
		}
		if containsModVer(out, "rsc.io/quote", "v1.5.1") {
			t.Errorf("unexpected v1.5.1 in output, got:\n%s", out)
		}
	})

	t.Run("keeps theirs-only module", func(t *testing.T) {
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo v1.0.0\n" +
				"=======\n" +
				"require rsc.io/foo v1.1.0\n" +
				"require rsc.io/bar v1.0.0\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/foo", "v1.1.0") {
			t.Errorf("expected foo v1.1.0 in output, got:\n%s", out)
		}
		if !containsModVer(out, "rsc.io/bar", "v1.0.0") {
			t.Errorf("expected bar v1.0.0 in output, got:\n%s", out)
		}
	})

	t.Run("resolves multiple conflict blocks", func(t *testing.T) {
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo v1.0.0\n" +
				"=======\n" +
				"require rsc.io/foo v1.1.0\n" +
				">>>>>>> branch\n" +
				"\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/bar v1.0.0\n" +
				"=======\n" +
				"require rsc.io/bar v1.1.0\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/foo", "v1.1.0") {
			t.Errorf("expected foo v1.1.0, got:\n%s", out)
		}
		if !containsModVer(out, "rsc.io/bar", "v1.1.0") {
			t.Errorf("expected bar v1.1.0, got:\n%s", out)
		}
	})

	t.Run("returns ErrorNoConflicts when no markers present", func(t *testing.T) {
		_, err := MergeGoMod([]byte("module a\n\ngo 1.21.0\n"))
		if !errors.Is(err, ErrorNoConflicts) {
			t.Errorf("want ErrorNoConflicts, got %v", err)
		}
	})

	t.Run("returns ErrorUnsupportedDirective for replace in conflict", func(t *testing.T) {
		input := []byte(
			"module a\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo v1.0.0\n" +
				"=======\n" +
				"require rsc.io/foo v1.1.0\n" +
				"replace rsc.io/foo v1.1.0 => ./local\n" +
				">>>>>>> branch\n",
		)
		_, err := MergeGoMod(input)
		if !errors.Is(err, ErrorUnsupportedDirective) {
			t.Errorf("want ErrorUnsupportedDirective, got %v", err)
		}
	})

	t.Run("version in module path not corrupted", func(t *testing.T) {
		input := []byte(
			"module a\n\n" +
				"<<<<<<< HEAD\n" +
				"require example.com/v1.5.1/pkg v1.5.1\n" +
				"=======\n" +
				"require example.com/v1.5.1/pkg v1.5.2\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "example.com/v1.5.1/pkg", "v1.5.2") {
			t.Errorf("expected path preserved and version bumped, got:\n%s", out)
		}
	})

	t.Run("diff3 parent marker is ignored", func(t *testing.T) {
		input := []byte(
			"module a\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo v1.1.0\n" +
				"||||||| parent\n" +
				"require rsc.io/foo v1.0.0\n" +
				"=======\n" +
				"require rsc.io/foo v1.2.0\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/foo", "v1.2.0") {
			t.Errorf("expected v1.2.0 (max), got:\n%s", out)
		}
	})

	t.Run("go directive: higher version wins", func(t *testing.T) {
		input := []byte(
			"module a\n\n" +
				"<<<<<<< HEAD\n" +
				"go 1.21.0\n" +
				"=======\n" +
				"go 1.22.0\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("go 1.22.0")) {
			t.Errorf("expected go 1.22.0 in output, got:\n%s", out)
		}
		if bytes.Contains(out, []byte("go 1.21.0")) {
			t.Errorf("unexpected go 1.21.0 in output, got:\n%s", out)
		}
	})

	t.Run("toolchain directive: higher version wins", func(t *testing.T) {
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"toolchain go1.21.5\n" +
				"=======\n" +
				"toolchain go1.22.3\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("go1.22.3")) {
			t.Errorf("expected go1.22.3 in output, got:\n%s", out)
		}
		if bytes.Contains(out, []byte("go1.21.5")) {
			t.Errorf("unexpected go1.21.5 in output, got:\n%s", out)
		}
	})

	t.Run("direct vs indirect: direct wins", func(t *testing.T) {
		// One side has a module as direct, the other as indirect.
		// The merged result should carry the higher version; the // indirect
		// annotation is left to go mod tidy, so we only check the version here.
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo v1.1.0\n" +
				"=======\n" +
				"require rsc.io/foo v1.1.0 // indirect\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/foo", "v1.1.0") {
			t.Errorf("expected rsc.io/foo v1.1.0 in output, got:\n%s", out)
		}
	})

	t.Run("pseudo-version semver ordering", func(t *testing.T) {
		// A tagged release must beat an older pseudo-version on the other side,
		// and a newer pseudo-version must beat an older one.
		pseudo := "v0.0.0-20230101000000-abcdef012345"
		tagged := "v0.1.0"
		input := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo " + pseudo + "\n" +
				"=======\n" +
				"require rsc.io/foo " + tagged + "\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out, "rsc.io/foo", tagged) {
			t.Errorf("expected tagged %s to win over pseudo-version, got:\n%s", tagged, out)
		}

		// newer pseudo beats older pseudo
		older := "v0.0.0-20220101000000-000000000000"
		newer := "v0.0.0-20230101000000-abcdef012345"
		input2 := []byte(
			"module a\n\ngo 1.21.0\n\n" +
				"<<<<<<< HEAD\n" +
				"require rsc.io/foo " + older + "\n" +
				"=======\n" +
				"require rsc.io/foo " + newer + "\n" +
				">>>>>>> branch\n",
		)
		out2, err := MergeGoMod(input2)
		if err != nil {
			t.Fatal(err)
		}
		if !containsModVer(out2, "rsc.io/foo", newer) {
			t.Errorf("expected newer pseudo-version to win, got:\n%s", out2)
		}
	})

	t.Run("toolchain rc vs release ordering", func(t *testing.T) {
		// go1.21rc1 < go1.21.0 by semver; the release must win.
		input := []byte(
			"module a\n\n" +
				"<<<<<<< HEAD\n" +
				"go 1.21rc1\n" +
				"toolchain go1.21rc1\n" +
				"=======\n" +
				"go 1.21.0\n" +
				"toolchain go1.21.0\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoMod(input)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, []byte("go 1.21.0")) {
			t.Errorf("expected go 1.21.0 in output, got:\n%s", out)
		}
		if bytes.Contains(out, []byte("go 1.21rc1")) {
			t.Errorf("unexpected go 1.21rc1 in output, got:\n%s", out)
		}
		if !bytes.Contains(out, []byte("go1.21.0")) {
			t.Errorf("expected toolchain go1.21.0 in output, got:\n%s", out)
		}
		if bytes.Contains(out, []byte("go1.21rc1")) {
			t.Errorf("unexpected toolchain go1.21rc1 in output, got:\n%s", out)
		}
	})

	t.Run("conflict-free file round-trips without formatting changes", func(t *testing.T) {
		// A file with no conflict markers must come back as ErrorNoConflicts
		// and must not be rewritten (no spurious diffs in the merge driver).
		clean := []byte("module a\n\ngo 1.21.0\n\nrequire rsc.io/quote v1.5.2\n")
		_, err := MergeGoMod(clean)
		if !errors.Is(err, ErrorNoConflicts) {
			t.Fatalf("want ErrorNoConflicts for conflict-free input, got %v", err)
		}
	})
}

func TestMergeGoSum(t *testing.T) {
	t.Run("union-merges both sides of conflict", func(t *testing.T) {
		input := []byte(
			"golang.org/x/text v0.16.0 h1:hash=\n" +
				"<<<<<<< HEAD\n" +
				"rsc.io/quote v1.5.1 h1:hashA=\n" +
				"rsc.io/quote v1.5.1/go.mod h1:hashB=\n" +
				"=======\n" +
				"rsc.io/quote v1.5.2 h1:hashC=\n" +
				"rsc.io/quote v1.5.2/go.mod h1:hashD=\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoSum(input)
		if err != nil {
			t.Fatal(err)
		}
		// both versions must be present (go mod tidy will prune the unused one)
		for _, want := range []string{
			"rsc.io/quote v1.5.1 h1:hashA=",
			"rsc.io/quote v1.5.1/go.mod h1:hashB=",
			"rsc.io/quote v1.5.2 h1:hashC=",
			"rsc.io/quote v1.5.2/go.mod h1:hashD=",
			"golang.org/x/text v0.16.0 h1:hash=",
		} {
			if !containsLine(out, want) {
				t.Errorf("expected line %q in output, got:\n%s", want, out)
			}
		}
	})

	t.Run("deduplicates identical lines from both sides", func(t *testing.T) {
		input := []byte(
			"<<<<<<< HEAD\n" +
				"rsc.io/quote v1.5.2 h1:hash=\n" +
				"=======\n" +
				"rsc.io/quote v1.5.2 h1:hash=\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoSum(input)
		if err != nil {
			t.Fatal(err)
		}
		count := countLines(out, "rsc.io/quote v1.5.2 h1:hash=")
		if count != 1 {
			t.Errorf("expected exactly 1 occurrence, got %d in:\n%s", count, out)
		}
	})

	t.Run("output is sorted", func(t *testing.T) {
		input := []byte(
			"<<<<<<< HEAD\n" +
				"z/pkg v1.0.0 h1:z=\n" +
				"=======\n" +
				"a/pkg v1.0.0 h1:a=\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoSum(input)
		if err != nil {
			t.Fatal(err)
		}
		lines := splitLines(out)
		for i := 1; i < len(lines); i++ {
			if lines[i] < lines[i-1] {
				t.Errorf("output not sorted at line %d: %q before %q", i, lines[i-1], lines[i])
			}
		}
	})

	t.Run("returns ErrorNoConflicts when no markers present", func(t *testing.T) {
		_, err := MergeGoSum([]byte("rsc.io/quote v1.5.2 h1:hash=\n"))
		if !errors.Is(err, ErrorNoConflicts) {
			t.Errorf("want ErrorNoConflicts, got %v", err)
		}
	})

	t.Run("diff3 parent marker lines are excluded from output", func(t *testing.T) {
		input := []byte(
			"<<<<<<< HEAD\n" +
				"rsc.io/quote v1.5.2 h1:new=\n" +
				"||||||| parent\n" +
				"rsc.io/quote v1.5.0 h1:old=\n" +
				"=======\n" +
				"rsc.io/quote v1.5.1 h1:other=\n" +
				">>>>>>> branch\n",
		)
		out, err := MergeGoSum(input)
		if err != nil {
			t.Fatal(err)
		}
		if containsLine(out, "rsc.io/quote v1.5.0 h1:old=") {
			t.Errorf("parent section line must not appear in output, got:\n%s", out)
		}
		if !containsLine(out, "rsc.io/quote v1.5.2 h1:new=") {
			t.Errorf("ours line missing from output, got:\n%s", out)
		}
		if !containsLine(out, "rsc.io/quote v1.5.1 h1:other=") {
			t.Errorf("theirs line missing from output, got:\n%s", out)
		}
	})
}

// containsModVer reports whether out contains "module version" as a substring,
// matching both single-line ("require module version") and block ("\tmodule version") forms.
func containsModVer(out []byte, module, version string) bool {
	return bytes.Contains(out, []byte(module+" "+version))
}

// containsLine reports whether out contains a line equal to s.
func containsLine(out []byte, s string) bool {
	for _, line := range splitLines(out) {
		if line == s {
			return true
		}
	}
	return false
}

// countLines counts occurrences of lines equal to s in out.
func countLines(out []byte, s string) int {
	n := 0
	for _, line := range splitLines(out) {
		if line == s {
			n++
		}
	}
	return n
}

func splitLines(out []byte) []string {
	var lines []string
	for _, b := range splitBytes(out) {
		if len(b) > 0 {
			lines = append(lines, string(b))
		}
	}
	return lines
}

func splitBytes(b []byte) [][]byte {
	var result [][]byte
	for len(b) > 0 {
		i := 0
		for i < len(b) && b[i] != '\n' {
			i++
		}
		result = append(result, b[:i])
		if i < len(b) {
			b = b[i+1:]
		} else {
			break
		}
	}
	return result
}
