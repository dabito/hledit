# Changelog

## [1.0.0] — 2026-06-22

### Changed

- Promote `hledit` CLI to stable 1.0.0 release.
- Align public CLI version with the first stable `pi-hledit` package release.

## [0.1.1] — 2026-06-21

### Changed

- **Pi extension: single `hledit` tool** — collapsed 5 separate tools (hledit_read, hledit_replace, hledit_replace_range, hledit_insert, hledit_batch) into one unified `hledit` tool with `op` parameter (read/edit/batch). Reduces token overhead and simplifies model usage.
- **Enriched error messages** — pi tool errors now include remediation hints with correct JSON format examples, valid op names, and anchor format guidance. Model can self-correct instead of guessing.
- **Batch edit input format** — simplified to JSON string with `anchor`/`end_anchor`/`lines` fields (consistent with single-edit param names).

## [0.1.0] — 2026-06-21

### Added

- `hledit` — hash-anchored line editor CLI for AI coding agents
- `read` / `read-range` — paginated file reading with LN#HASH anchors
- `replace` / `replace-range` / `insert` — stale-safe edit operations
- `batch` — multi-edit atomic operations (validates all anchors, applies bottom-up, single write)
- `--grep` flag — filter lines by substring match for token-efficient targeted reads
- `--version` / `version` — print version and exit
- Atomic writes (temp file + rename) with original file permission preservation
- Trailing newline preservation across all edit operations
- `pi-hledit` pi coding agent extension
- 22 golden integration tests covering all operations and edge cases
- Comprehensive unit test suite
- CHANGELOG.md, LICENSE (MIT), Makefile, ROADMAP.md
