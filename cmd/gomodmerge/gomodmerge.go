package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mauri870/gomodmerge/internal/gomod"
	"github.com/mauri870/gomodmerge/internal/mergefix"
)


func main() {
	if len(os.Args) < 2 {
		// no arguments: fix conflicts in the current directory
		if err := fixAll(); err != nil {
			fmt.Fprintf(os.Stderr, "gomodmerge: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// called by git as a merge driver: gomodmerge %A %O %B %P
	if len(os.Args) < 5 {
		fmt.Fprintln(os.Stderr, "usage: gomodmerge %A %O %B %P  (invoked by git)")
		os.Exit(1)
	}
	current, base, other, fname := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
	if err := mergeDriver(current, base, other, fname); err != nil {
		fmt.Fprintf(os.Stderr, "gomodmerge: %v\n", err)
		os.Exit(1)
	}
}

// fixAll fixes conflicts in go.mod and go.sum in the current directory.
func fixAll() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %v", err)
	}

	for _, file := range []string{"go.mod", "go.sum"} {
		if err := fixFile(filepath.Join(dir, file)); err != nil {
			return err
		}
	}

	if err := gomod.Tidy(dir); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %v", err)
	}
	return nil
}

func fixFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s", filepath.Base(filename))
		}
		return fmt.Errorf("failed to open %s: %v", filepath.Base(filename), err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v", filepath.Base(filename), err)
	}

	mergeFunc := mergeFuncFor(filepath.Base(filename))
	if mergeFunc == nil {
		return fmt.Errorf("unsupported file: %s", filepath.Base(filename))
	}

	out, err := mergeFunc(b)
	if err != nil {
		switch {
		case errors.Is(err, mergefix.ErrorNoConflicts):
			return nil
		case errors.Is(err, mergefix.ErrorUnsupportedDirective):
			return fmt.Errorf("replace or exclude directives found. Please fix the conflicts manually.")
		default:
			return fmt.Errorf("failed to fix conflicts in %s: %v", filepath.Base(filename), err)
		}
	}

	f.Close()
	if err := os.WriteFile(filename, out, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %v", filepath.Base(filename), err)
	}

	fmt.Printf("gomodmerge: %s merged\n", filepath.Base(filename))
	return nil
}

// mergeDriver is invoked by git with the %A %O %B %P arguments.
func mergeDriver(current, base, other, fname string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	cmd := exec.Command("git", "merge-file", "-p", current, base, other)
	out, _ := cmd.Output() // ignore exit code; conflicts are expected

	mergeFunc := mergeFuncFor(filepath.Base(fname))
	if mergeFunc == nil {
		return fmt.Errorf("unsupported file: %s", fname)
	}

	buf, err := mergeFunc(out)
	if err != nil {
		if errors.Is(err, mergefix.ErrorNoConflicts) {
			buf = out
		} else {
			return fmt.Errorf("failed to fix conflicts: %v", err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, fname), buf, 0644); err != nil {
		return err
	}

	if err := gomod.Tidy(dir); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %v", err)
	}

	buf, err = os.ReadFile(filepath.Join(dir, fname))
	if err != nil {
		return err
	}
	return os.WriteFile(current, buf, 0644)
}

func mergeFuncFor(base string) func([]byte) ([]byte, error) {
	switch base {
	case "go.mod":
		return mergefix.MergeGoMod
	case "go.sum":
		return mergefix.MergeGoSum
	}
	return nil
}

