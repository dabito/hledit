# Roadmap

`hledit` is intentionally small right now: one Go binary, stdlib-only, and a simple local build/install workflow.

## Current release workflow

For development and local use:

```bash
make build
make install
```

This produces:

```text
dist/hledit
~/.local/bin/hledit -> <repo>/dist/hledit
```

`LOCAL_BIN` can be overridden:

```bash
make install LOCAL_BIN="$HOME/bin"
```

The `dist/` directory and root `hledit` binary are ignored by git. We do not commit compiled binaries.

## Deferred: GoReleaser

We are deliberately **not** adding GoReleaser yet.

Reasons:

1. **No public release cadence yet** — the tool is still exploratory and local-first.
2. **Single binary, no assets** — `go build` plus a tiny `Makefile` is enough.
3. **No versioning/tag policy yet** — GoReleaser is most useful once tags, changelogs, archives, checksums, and GitHub releases matter.
4. **Avoid configuration drag** — adding `.goreleaser.yaml` now would create maintenance surface before we need it.

## When to add GoReleaser

Add GoReleaser when at least one of these is true:

- We want GitHub Releases with downloadable archives.
- We need cross-platform builds for `darwin/linux/windows` and `amd64/arm64`.
- We want generated checksums.
- We want Homebrew tap support.
- We want signed artifacts or provenance metadata.
- We publish versioned binaries for other people to install.

## Likely future release matrix

When we do add release automation, start with:

```text
darwin/arm64
darwin/amd64
linux/arm64
linux/amd64
windows/amd64
```

Possible artifact names:

```text
hledit_Darwin_arm64.tar.gz
hledit_Darwin_x86_64.tar.gz
hledit_Linux_arm64.tar.gz
hledit_Linux_x86_64.tar.gz
hledit_Windows_x86_64.zip
checksums.txt
```

## Agent search and anchor safety backlog

- [x] Expose grep context windows through all wrappers; default to a small balanced context (`2`) when `grep` is used, and allow `context:0` for match-only output.
- [x] Add anchored repo-wide `find` with bounded output, include/exclude globs, default ignore support, and optional context windows.
- [ ] Add `peek`/read-around-anchor support so agents can expand context from a known line or `LN#HASH` without manual offset math.
- [ ] Add better match controls: `ignoreCase`, `word`, `maxMatches`, then explicit regex mode once escaping/schema risks are handled.
- [ ] Add variable-length hashes (`--hash-len` / env) and accept 2-6 character anchors; keep 3-char default until a versioned default change is justified.
- [ ] Consider adaptive hash length for risky structural lines only after fixed-length support exists and agent output impact is measured.

## Near-term roadmap

- Keep the `Makefile` workflow simple.
- Keep tests fully offline and deterministic.
- Consider shell completions (`hledit completion zsh|bash|fish`) now that `anchors` exists.
- Revisit GoReleaser once the module has a public repository path and version tags.

- Consider splitting `edit.go` into `helpers.go` + `batch.go` once docs/release work settles; current mixed file is publishable with strong contract tests, but the split keeps command logic easier to audit.
