package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"

	"github.com/mauri870/gosumfix/internal/gomod"
	"github.com/mauri870/gosumfix/internal/mergefix"
)

var (
	gitAddNameCmd = []string{
		"git", "config", "--global", "merge.gosumdriver.name",
		"A custom merge driver to fix go.mod and go.sum conflicts",
	}
	gitAddDriverCmd = []string{
		"git", "config", "--global", "merge.gosumdriver.driver",
		"gosumdriver %A %O %B %P",
	}
	driverInstalledMsg = `gosumdriver installed successfully

Please add the following lines to your .gitattributes file:

	go.mod merge=gosumdriver
	go.sum merge=gosumdriver

You can find the .gitattributes file with the following command:

	git config core.attributesfile

If the previous command returns an empty string, you can create a global
.gitattributes in your HOME directory and add the above lines to it:

	echo "go.mod merge=gosumdriver\ngo.sum merge=gosumdriver" > ~/.gitattributes
	git config core.attributesfile ~/.gitattributes

Run 'gosumdriver uninstall' to remove the driver.
`
	gitRemoveDriverCmd = []string{
		"git", "config", "--global", "--remove-section", "merge.gosumdriver",
	}
	driverUninstalledMsg = `gosumdriver uninstalled successfully

Please manually remove the following lines from your .gitattributes file:

	go.mod merge=gosumdriver
	go.sum merge=gosumdriver
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: gosumdriver [install|uninstall]")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "uninstall":
		if err := uninstall(); err != nil {
			fmt.Printf("failed to uninstall merge driver: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(driverUninstalledMsg)
		return
	case "install":
		if err := install(); err != nil {
			fmt.Printf("failed to install merge driver: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(driverInstalledMsg)
		return
	}

	if len(os.Args) < 5 {
		fmt.Println("usage: gosumdriver %A %O %B %P")
		fmt.Println("This command should be run only by git. Do not run it manually.")
		os.Exit(1)
	}

	current := os.Args[1]
	base := os.Args[2]
	other := os.Args[3]
	fname := os.Args[4]

	if err := merge(current, base, other, fname); err != nil {
		fmt.Printf("failed to merge: %v\n", err)
		os.Exit(1)
	}
}

func install() error {
	cmd := exec.Command(gitAddNameCmd[0], gitAddNameCmd[1:]...)
	if err := cmd.Run(); err != nil {
		return err
	}

	cmd = exec.Command(gitAddDriverCmd[0], gitAddDriverCmd[1:]...)
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func uninstall() error {
	cmd := exec.Command(gitRemoveDriverCmd[0], gitRemoveDriverCmd[1:]...)
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func merge(current, base, other, fname string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %v", err)
	}

	// create a merge file with the conflicts
	cmd := exec.Command("git", "merge-file", "-p", current, base, other)

	// ignore merge errors, we will fix them next
	out, _ := cmd.Output()

	// select merge strategy based on file type
	var mergeFunc func([]byte) ([]byte, error)
	switch path.Base(fname) {
	case "go.mod":
		mergeFunc = mergefix.MergeGoMod
	case "go.sum":
		mergeFunc = mergefix.MergeGoSum
	default:
		return fmt.Errorf("unsupported file: %s", fname)
	}

	// fix conflicts; if git already resolved them cleanly, use the output as-is
	buf, err := mergeFunc(out)
	if err != nil {
		if errors.Is(err, mergefix.ErrorNoConflicts) {
			buf = out
		} else {
			return fmt.Errorf("failed to fix conflicts: %v", err)
		}
	}

	// save the file overwriting the original file
	err = os.WriteFile(path.Join(dir, fname), buf, 0644)
	if err != nil {
		return err
	}

	// reconcile dependencies
	if err := gomod.Tidy(dir); err != nil {
		return fmt.Errorf("failed to run go mod tidy: %v", err)
	}

	// save the merged file as the current file
	buf, err = os.ReadFile(path.Join(dir, fname))
	if err != nil {
		return err
	}
	err = os.WriteFile(current, buf, 0644)
	if err != nil {
		return err
	}

	return nil
}
