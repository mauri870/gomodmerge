# gomodmerge

`gomodmerge` automatically resolves git merge conflicts in `go.mod` and `go.sum` files.

> Upstream proposal for a git merge tool for cmd/go [golang/go#32485](https://github.com/golang/go/issues/32485).

## How It Works

**`go.sum`** conflicts are resolved by taking the union of both sides, deduplicating entries, and sorting the result. This is always safe because `go.sum` is an append-only set of content hashes.

**`go.mod`** conflicts are resolved semantically using [`golang.org/x/mod/modfile`](https://pkg.go.dev/golang.org/x/mod/modfile). Naively concatenating both sides is incorrect; gomodmerge instead applies proper merge semantics:

- `require` directives: the higher semver version wins per module (consistent with MVS); modules present only in one side are kept.
- `go` and `toolchain` directives: the higher version wins.
- `replace` and `exclude` directives inside conflict blocks cannot be auto-resolved and require manual intervention; gomodmerge exits non-zero in that case.

After resolving conflicts, `go mod tidy` is run automatically to reconcile the dependency graph.

## Installation

### Per-project via `go tool` (Go 1.24+)

Add the tool as a dependency of your module:

```bash
go get -tool github.com/mauri870/gomodmerge/cmd/gomodmerge@latest
```

This records the tool in `go.mod` so anyone who checks out the repo can use it without a separate install step.

```bash
go tool gomodmerge
```

### Global

```bash
go install github.com/mauri870/gomodmerge/cmd/gomodmerge@latest
```

## Usage

### Manual

Run from the root of the repository after a conflicted merge:

```bash
gomodmerge          # global install
go tool gomodmerge  # go tool install
```

It will resolve conflicts in `go.mod` and `go.sum` and run `go mod tidy`.

### Git Merge Driver (automatic)

Install the driver once globally:

```bash
gomodmerge install          # global install
go tool gomodmerge install  # go tool install
```

Then add the following lines to your `.gitattributes` file (global or per-repo):

```
go.mod merge=gomodmerge
go.sum merge=gomodmerge
```

To find or create a global `.gitattributes` file:

```bash
# Check if one is already configured
git config core.attributesfile

# If not, create one in your home directory
echo "go.mod merge=gomodmerge" >> ~/.gitattributes
echo "go.sum merge=gomodmerge" >> ~/.gitattributes
git config --global core.attributesfile ~/.gitattributes
```

Once installed, `git merge` and `git rebase` will automatically invoke `gomodmerge` whenever `go.mod` or `go.sum` conflicts are detected.

To uninstall the driver:

```bash
gomodmerge uninstall          # global install
go tool gomodmerge uninstall  # go tool install
```

Then remove the `merge=gomodmerge` lines from your `.gitattributes` file.
