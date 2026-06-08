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

**Limitations:** `go.work` and `go.work.sum` workspace files are not supported.

## Manual usage

Install:

```bash
go get -tool github.com/mauri870/gomodmerge/cmd/gomodmerge@latest  # per-project (recommended, Go 1.24+)
go install github.com/mauri870/gomodmerge/cmd/gomodmerge@latest    # global
```

Run from the root of the repository after a conflicted merge:

```bash
go tool gomodmerge  # per-project
gomodmerge          # global
```

## Git Merge Driver

The driver makes `git merge` and `git rebase` resolve conflicts automatically.

**1. Wire up `.gitattributes`**

Per-repo: commit this file to the repository:

```
go.mod merge=go tool gomodmerge
go.sum merge=go tool gomodmerge
```

Or host-wise:

```bash
echo "go.mod merge=gomodmerge" >> ~/.gitattributes
echo "go.sum merge=gomodmerge" >> ~/.gitattributes
git config --global core.attributesfile ~/.gitattributes
```

**2. Register the driver**:

```bash
go tool gomodmerge install  # per-project
gomodmerge install          # global
```

To uninstall, run `go tool gomodmerge uninstall` (or `gomodmerge uninstall`) and remove the `merge=gomodmerge` lines from your `.gitattributes`.
