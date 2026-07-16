# hledit — Spec

## 1. Binary & Invocation

```
hledit <verb> [flags] <file> [anchor] [end-anchor] <content-source>
```

- Logical outcomes (success, stale anchors, invalid anchors/content, binary/range/io errors) exit 0 and are reported on stdout.
- CLI misuse exits 2 with usage on stderr; unrecoverable infrastructure failures exit 1.

## 2. Verbs

### 2.1 `read`

```
hledit read <file> [--grep <pattern>] [--context N] [--json]
```

Reads the entire file. Each line is emitted as:

```
<LN>#<HASH>:<content>
```

- `LN` — 1-indexed line number.
- `HASH` — 3-character hash (see §3). Legacy 2-character hashes are accepted for write anchors.
- `:` — literal separator.
- Content includes the original line without trailing `\n` or `\r`.
- `--grep` — substring match; only matching lines are emitted.
- `--context` — include N lines before/after each match; overlapping windows merge.
- `--json` — emit JSON `{ok, lines:[{line,anchor,text}], truncated, nextOffset}`.

**Truncation:** Stop at 50 KB of output or 2,000 lines, whichever is first. Append a trailing line:

```
-- truncated: use read-range --offset <next> --
```

**Binary detection:** If the file is detected as binary (contains NUL byte in first 8 KB), emit:

```json
{ "ok": false, "error": "binary", "message": "file appears to be binary" }
```

### 2.2 `read-range`

```
hledit read-range <file> [--offset <N>] [--limit <M>] [--grep <pattern>] [--context N] [--json]
```

- `--offset` — 1-indexed starting line (default 1).
- `--limit` — max lines to return (default 2000).
- `--grep` — substring match; only matching lines are emitted.
- `--context` — include N lines before/after each match; overlapping windows merge.

Same output format as `read`. Same truncation behavior at 50 KB / 2,000 lines from the offset.
- `--json` — same JSON shape.

If `--offset` exceeds file length, emit:

```json
{ "ok": false, "error": "range", "message": "offset 500 exceeds file length 120" }
```

### 2.3 `anchors`

```
hledit anchors <file> [--offset <N>] [--limit <M>] [--grep <pattern>] [--context N] [--json]
```

- Same flags and filtering as `read-range`.
- Emits `ANCHOR<TAB>TEXT` instead of `LN#HASH:TEXT`.
- Same truncation behavior at 50 KB / 2,000 lines from the offset.
- `--json` — same JSON shape.

If `--offset` exceeds file length, emit:

```json
{ "ok": false, "error": "range", "message": "offset 500 exceeds file length 120" }
```

### 2.4 `find`

```
hledit find <pattern> [path] [--context N] [--limit N] [--max-files N] [--include glob] [--exclude glob] [--json] [--pretty]
```

Recursively searches `path` (default `.`) using substring, case-sensitive matching. Regex is not enabled.

Default text output is grouped by slash-normalized relative file path:

```
src/auth.ts
42#NKA:function validateToken(token: string) {
```

Anchors are file-agnostic; an anchor returned by `find` must be used with its file path. `--json` is safer for agents/wrappers because it preserves `{file, lines:[{line,anchor,text}]}` pairing:

```json
{
  "ok": true,
  "mode": "substring",
  "matches": [
    { "file": "src/auth.ts", "lines": [ { "line": 42, "anchor": "42#NKA", "text": "function validateToken(token: string) {" } ] }
  ],
  "truncated": false,
  "filesSearched": 128,
  "filesSkipped": 6,
  "matchCount": 1,
  "emittedLineCount": 1
}
```

- `--context` — include N lines before/after each match; overlapping windows merge.
- `--limit` — maximum matching/context lines to emit (default 500).
- `--max-files` — maximum matching file groups to emit (default 100).
- `--include` / `--exclude` — repeatable globs matched against slash-normalized relative paths; when a glob has no slash it also matches basenames; directory excludes prune matching trees; exclude wins over include.
- Default ignored directories: `.git`, `node_modules`, `dist`, `build`, `coverage`, `.cache`, `.next`, `.nuxt`, `target`, `.venv`, `__pycache__`, `vendor`.
- Symlinks, binary files, and files over 2 MiB are skipped and counted in `filesSkipped`.
- Text is bounded by line count, matching file count, a global byte budget, and a per-line display cap. A displayed line may be truncated while its anchor still hashes the full original line.
- Zero matches emit no text, or JSON with `matches:[]`; exit code remains 0.
### 2.5 `replace`

```
hledit replace <file> <anchor> <content-source>
```

- `anchor` — `LN#HASH` targeting a single line.
- `content-source` — `-` for stdin, or a file path.
- Reads replacement content from the source (one or more lines).
- If content is empty, the line is **deleted**.
- If content has multiple lines, the single targeted line is replaced with all of them (net insert).

**Behavior:**

1. Validate anchor against current file.
2. If hash mismatches, return stale error (see §5).
3. Replace the line at `LN` with the new content.
4. Write atomically (temp + rename).

### 2.6 `replace-range`

```
hledit replace-range <file> <anchor> <end-anchor> <content-source>
```

- `anchor` — start `LN#HASH` (inclusive).
- `end-anchor` — end `LN#HASH` (inclusive).
- Replaces all lines from `anchor.Line` through `end-anchor.Line` with the new content.
- If content is empty, the range is **deleted**.

**Validation:**

- `anchor.Line <= end-anchor.Line`.
- Both anchors must match current file hashes.

### 2.7 `insert`

```
hledit insert [--before|--after] <file> <anchor> <content-source>
```

- `--before` (default) — insert lines before the anchored line.
- `--after` — insert lines after the anchored line.
- Anchor is used **only for validation**, not for replacement. The anchored line stays untouched.
- Content must be non-empty.

**Behavior:**

1. Validate anchor against current file.
2. Insert new lines at the specified position.
3. Write atomically.

### 2.8 `batch`

```
hledit batch [--check] <file>
```

Reads a JSON `BatchEditRequest` from stdin:
`--check` validates stdin JSON, anchors, and ops without writing; success adds `checked:true`.

```json
{
  "edits": [
    { "op": "replace", "pos": "12#NKA", "lines": ["new line"] },
    { "op": "replace", "pos": "12#NKA", "end_pos": "18#VRC", "lines": ["new block"] },
    { "op": "delete", "pos": "5#TXA", "lines": [] },
    { "op": "insert", "pos": "8#VRB", "lines": ["inserted"] }
  ]
}
```

Validation:

- All anchors are validated against the original file state before any write.
- `replace` and `delete` use optional `end_pos` as an inclusive range end; if omitted, they target only `pos`.
- `replace` and `delete` require `pos.Line <= end_pos.Line` when `end_pos` is provided.
- `insert` requires non-empty `lines` and inserts before `pos`.
- Unknown operations or invalid anchors return `error: "invalid"`; stale anchors return `error: "stale"` with remaps.

Application:

- Edits are applied bottom-up by original `pos.Line`.
- The file is written once, atomically, only after the full batch validates.
## 3. Hash Algorithm

```
computeLineHash(lineNum, line):
  1. line = trimRight(line, '\r')
  2. line = trimRight(line, whitespace)
  3. h = FNV-1a-32()
  4. if line has NO letter AND NO digit:
       mix lineNum into FNV-1a state before content
  5. h.write(line)
  6. sum = h.sum32()
  7. return sum encoded as 3 base32 chars
```

**Default alphabet:** `ABCDEFGHJKLMNPQRSTUVWXYZ23456789` (32 chars; drops `0/O` and `1/I`, keeps `L`). Legacy 2-character anchors use the old `ZPMQVRWSNKTXJBYH` alphabet and are accepted for writes.

**Line-number mixing** (step 3): Write the line number as a varint-style sequence of bytes (little-endian, stopping at first zero high byte) into the hash state before the line content. This ensures structurally identical non-significant lines (e.g. two blank lines, or `}` at different positions) produce different hashes.

**Detection of "significant" lines:** A line is significant if it contains at least one Unicode letter (`IsLetter`) or one Unicode digit (`IsDigit`). Blank lines, `{`, `}`, `),` etc. are non-significant.

## 4. Edit Application

### 4.1 Batch semantics

Every write invocation validates all anchors and content before writing. If any anchor is stale or any operation is invalid, nothing is written.

### 4.2 Application order

Single-edit verbs apply one operation. `batch` applies validated edits bottom-up (highest original line number first) so earlier line-number references are not shifted by later edits.
### 4.3 Atomic writes

1. Write new content to `<file>.hledit.tmp`.
2. `os.Rename` the temp file over the original.
3. If the process dies mid-write, the original file is intact (rename is atomic on POSIX).

## 5. Stale Detection & Error Response

When any anchor's hash doesn't match the current file content:

```json
{
  "ok": false,
  "error": "stale",
  "remaps": [
    { "requested": "5#TXA", "current": "5#NKA" },
    { "requested": "8#QRB", "current": "9#VRB" }
  ],
  "message": "anchor 5#TXA: expected hash TXA, got NKA"
}
```

- `remaps` maps every stale anchor to its current correct anchor.
- The agent can retry immediately using the remapped anchors without re-reading the file.
- The whole edit is rejected — no partial writes.

## 6. Success Response

Single writes may include `lastChangedLine`:

```json
{ "ok": true, "firstChangedLine": 5, "lastChangedLine": 5 }
```

Batch writes include `firstChangedLine`, `lastChangedLine`, `editsApplied`; `--check` also includes `checked:true`.

## 7. Content Source

The `<content-source>` argument:

| Value | Meaning |
|---|---|
| `-` | Read content from stdin |
| Any other path | Read content from that file |

Content is read as-is, split by `\n`. Trailing `\n` on the last line is stripped (does not introduce an extra empty line). `\r\n` is normalized to `\n`.

For `replace` / `replace-range` with empty content, the effect is deletion.

For `insert`, content must be non-empty. Empty content returns:

```json
{ "ok": false, "error": "invalid", "message": "insert requires non-empty content" }
```

## 8. Anchor Parsing

Anchors match the regex:

```
^\s*(\d+)\s*#\s*([ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{3}|[ZPMQVRWSNKTXJBYH]{2})(?:\b|[^A-Za-z0-9])
```

Lenient: tolerates surrounding whitespace. If a model copy-pastes a full annotated line like `5#TXA:func main() {`, the parser extracts `5#TXA` and ignores the rest. Legacy 2-character anchors like `5#TX` are also accepted.

Invalid anchors return:

```json
{ "ok": false, "error": "invalid", "message": "invalid anchor \"foo\": expected LN#HASH or legacy LN#HH" }
```

## 9. Exit Codes

| Code | Meaning |
|---|---|
| 0 | Normal — check JSON `ok` for success vs logical error |
| 1 | Unrecoverable I/O error (file not found, permission denied, etc.) |

Exit code 1 is only for infrastructure failures. All logical errors (stale, invalid anchor, empty content) return exit 0 with `ok: false` in JSON.

## 10. File Layout

```
.
├── main.go          # Entry point, CLI dispatch
├── read.go          # read + read-range + anchors verbs
├── edit.go          # replace, replace-range, insert verbs
├── hash.go          # FNV-1a hash, base32 alphabet, formatTag
├── types.go         # Anchor, EditResult, EditError, response types
├── anchor.go        # Anchor parsing + validation
├── write.go         # Atomic write logic (temp + rename)
├── go.mod
├── PRD.md
└── SPEC.md
```
