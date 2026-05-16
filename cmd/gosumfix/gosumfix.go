package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/mauri870/gosumfix/internal/gomod"
	"github.com/mauri870/gosumfix/internal/mergefix"
)

func main() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("gosumfix: failed to get current working directory: %v\n", err)
		os.Exit(1)
	}

	files := []string{"go.mod", "go.sum"}
	for _, file := range files {
		if err := fixConflicts(path.Join(dir, file)); err != nil {
			fmt.Printf("gosumfix: %v\n", err)
			os.Exit(1)
		}
	}

	if err := gomod.Tidy(dir); err != nil {
		fmt.Printf("gosumfix: failed to run go mod tidy: %v\n", err)
		os.Exit(1)
	}
}

func fixConflicts(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file not found: %s\n", path.Base(filename))
		}
		return fmt.Errorf("failed to open %s: %v\n", path.Base(filename), err)
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read %s: %v\n", path.Base(filename), err)
	}

	var mergeFunc func([]byte) ([]byte, error)
	switch path.Base(filename) {
	case "go.mod":
		mergeFunc = mergefix.MergeGoMod
	case "go.sum":
		mergeFunc = mergefix.MergeGoSum
	default:
		return fmt.Errorf("unsupported file: %s\n", path.Base(filename))
	}

	out, err := mergeFunc(b)
	if err != nil {
		switch {
		case errors.Is(err, mergefix.ErrorNoConflicts):
			return nil
		case errors.Is(err, mergefix.ErrorUnsupportedDirective):
			return fmt.Errorf("replace or exclude directives found. Please fix the conflicts manually.\n")
		default:
			return fmt.Errorf("failed to fix conflicts in %s: %v\n", path.Base(filename), err)
		}
	}

	f.Close()
	err = os.WriteFile(filename, out, 0644)
	if err != nil {
		return fmt.Errorf("failed to write %s: %v\n", path.Base(filename), err)
	}

	fmt.Printf("gosumfix: %s merged\n", path.Base(filename))
	return nil
}
