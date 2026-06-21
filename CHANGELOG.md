# Changelog

## [Unreleased]

### Added

- `hledit` — hash-anchored line editor CLI for AI coding agents
- `read` / `read-range` — paginated file reading with LN#HASH anchors
- `replace` / `replace-range` / `insert` — stale-safe edit operations
- Atomic writes (temp file + rename) with original file permission preservation
- Trailing newline preservation across all edit operations
- `pi-hledit` pi extension for AI tool integration
- Golden integration test with official `uuid@14.0.0` fixture
- Full unit test coverage (97.5%)
