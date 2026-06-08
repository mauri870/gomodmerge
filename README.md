# gosumfix

`gosumfix` automatically resolves git merge conflicts in `go.mod` and `go.sum` files.

> There is an upstream proposal for a git merge tool for cmd/go in [golang/go#32485](https://github.com/golang/go/issues/32485).

## How It Works

**`go.sum`** conflicts are resolved by taking the union of both sides, deduplicating entries, and sorting the result. This is always safe because `go.sum` is an append-only set of content hashes.

**`go.mod`** conflicts are resolved semantically using [`golang.org/x/mod/modfile`](https://pkg.go.dev/golang.org/x/mod/modfile):

- `require` directives: the higher semver version wins per module; modules present only in one side are kept.
- `go` and `toolchain` directives: the higher version wins.
- `replace` and `exclude` directives inside conflict blocks are not supported and must be resolved manually.

After resolving conflicts, `go mod tidy` is run automatically to reconcile the dependency graph.

## Installation

```bash
go install github.com/mauri870/gosumfix/cmd/...@latest
```

This installs two binaries into `$GOPATH/bin`:

- `gosumfix`: Manually resolve conflicts in the current directory
- `gosumdriver`: Git merge driver that invokes `gosumfix` automatically

## Usage

### Manual

Run `gosumfix` from the root of the repository after a conflicted merge:

```bash
gosumfix
```

It will resolve conflicts in `go.mod` and `go.sum` and run `go mod tidy`.

### Git Merge Driver (automatic)

Install the driver once globally:

```bash
gosumdriver install
```

Then add the following lines to your `.gitattributes` file (global or per-repo):

```
go.mod merge=gosumdriver
go.sum merge=gosumdriver
```

To find or create a global `.gitattributes` file:

```bash
# Check if one is already configured
git config core.attributesfile

# If not, create one in your home directory
echo "go.mod merge=gosumdriver" >> ~/.gitattributes
echo "go.sum merge=gosumdriver" >> ~/.gitattributes
git config --global core.attributesfile ~/.gitattributes
```

Once installed, `git merge` and `git rebase` will automatically invoke `gosumfix` whenever `go.mod` or `go.sum` conflicts are detected.

To uninstall the driver:

```bash
gosumdriver uninstall
```

Then remove the `merge=gosumdriver` lines from your `.gitattributes` file.
