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

var (
	gitAddNameCmd = []string{
		"git", "config", "--global", "merge.gomodmerge.name",
		"A custom merge driver to fix go.mod and go.sum conflicts",
	}
	gitAddDriverCmd = []string{
		"git", "config", "--global", "merge.gomodmerge.driver",
		"gomodmerge %A %O %B %P",
	}
	driverInstalledMsg = `gomodmerge driver installed successfully

Please add the following lines to your .gitattributes file:

	go.mod merge=gomodmerge
	go.sum merge=gomodmerge

You can find the .gitattributes file with the following command:

	git config core.attributesfile

If the previous command returns an empty string, you can create a global
.gitattributes in your HOME directory and add the above lines to it:

	echo "go.mod merge=gomodmerge" >> ~/.gitattributes
	echo "go.sum merge=gomodmerge" >> ~/.gitattributes
	git config --global core.attributesfile ~/.gitattributes

Run 'gomodmerge uninstall' to remove the driver.
`
	gitRemoveDriverCmd = []string{
		"git", "config", "--global", "--remove-section", "merge.gomodmerge",
	}
	driverUninstalledMsg = `gomodmerge driver uninstalled successfully

Please manually remove the following lines from your .gitattributes file:

	go.mod merge=gomodmerge
	go.sum merge=gomodmerge
`
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

	// git merge driver
	switch os.Args[1] {
	case "install":
		if err := install(); err != nil {
			fmt.Fprintf(os.Stderr, "gomodmerge: failed to install merge driver: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(driverInstalledMsg)

	case "uninstall":
		if err := uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "gomodmerge: failed to uninstall merge driver: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(driverUninstalledMsg)

	default:
		// called by git as a merge driver: gomodmerge %A %O %B %P
		if len(os.Args) < 5 {
			fmt.Fprintln(os.Stderr, "usage: gomodmerge [install|uninstall]")
			fmt.Fprintln(os.Stderr, "       gomodmerge %A %O %B %P  (invoked by git)")
			os.Exit(1)
		}
		current, base, other, fname := os.Args[1], os.Args[2], os.Args[3], os.Args[4]
		if err := mergeDriver(current, base, other, fname); err != nil {
			fmt.Fprintf(os.Stderr, "gomodmerge: %v\n", err)
			os.Exit(1)
		}
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

func install() error {
	for _, args := range [][]string{gitAddNameCmd, gitAddDriverCmd} {
		if err := exec.Command(args[0], args[1:]...).Run(); err != nil {
			return err
		}
	}
	return nil
}

func uninstall() error {
	return exec.Command(gitRemoveDriverCmd[0], gitRemoveDriverCmd[1:]...).Run()
}
