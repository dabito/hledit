# go-hashline-edit

`hledit` is a tiny hash-anchored line editor for AI coding agents.

Instead of asking an agent to reproduce old text exactly, `hledit read` annotates each line with a stable anchor:

```text
5#HY:func main() {
6#MX:    fmt.Println("hello")
7#NP:}
```

Write commands reference anchors such as `6#MX`. Before changing the file, `hledit` recomputes the hash at that line. If the file changed since it was read, the anchor is rejected and no write happens.

## Install

For local development, build into `dist/` and symlink into `~/.local/bin`:

```bash
make install
```

Override the target bin directory if needed:

```bash
make install LOCAL_BIN="$HOME/bin"
```

Or use standard Go install:

```bash
go install .
```

Build without installing:

```bash
make build
# writes dist/hledit
```

## Pi integration

This repo includes a pi extension in [`pi-hledit/`](./pi-hledit/) that exposes the binary as tools:

- `hledit_read`
- `hledit_replace`
- `hledit_replace_range`
- `hledit_insert`

For this machine it is symlinked into pi's global extension directory:

```bash
~/.pi/agent/extensions/pi-hledit -> ./pi-hledit
```

After changing the extension, reload pi:

```text
/reload
```

The extension uses `~/.local/bin/hledit` by default. Override with:

```bash
export HLEDIT_BIN=/path/to/hledit
```

## Commands

```text
hledit read <file>
hledit read-range <file> [--offset N] [--limit M]
hledit replace <file> <anchor> <content-source>
hledit replace-range <file> <anchor> <end-anchor> <content-source>
hledit insert [--before|--after] <file> <anchor> <content-source>
```

`<content-source>` is either `-` for stdin or a file path.

## Examples

Read a file:

```bash
hledit read main.go
```

Read a window of a large file:

```bash
hledit read-range main.go --offset 40 --limit 20
```

Replace one line using stdin:

```bash
printf '    fmt.Println("hello world")\n' | hledit replace main.go 6#MX -
```

Replace a range using a prepared file:

```bash
hledit replace-range main.go 12#NK 18#VR /tmp/new-block.txt
```

Insert before or after an anchor:

```bash
cat header.txt | hledit insert --before main.go 1#WV -
printf '// done\n' | hledit insert --after main.go 99#TX -
```

Delete a line or range by providing empty content:

```bash
: | hledit replace main.go 6#MX -
: | hledit replace-range main.go 12#NK 18#VR -
```

## Output

Read commands emit annotated text:

```text
1#WV:package main
2#VR:
3#JB:import "fmt"
```

Write commands emit JSON:

```json
{"ok":true,"firstChangedLine":6}
```

Stale anchors are rejected atomically:

```json
{
  "ok": false,
  "error": "stale",
  "message": "anchor 6#MX: stale",
  "remaps": [{"requested":"6#MX","current":"6#MQ"}]
}
```

## Hash format

Anchors are `LN#HH`:

- `LN` is the 1-indexed line number.
- `HH` is a 2-character content hash.
- The hash uses FNV-1a 32-bit, normalized trailing whitespace, and the alphabet `ZPMQVRWSNKTXJBYH`.
- Blank or punctuation-only lines mix the line number into the hash so identical structural lines are easier for models to distinguish.

## Behavior notes

- Writes are atomic: temp file + rename.
- All anchors in a write are validated before writing.
- Logical failures (`stale`, `invalid`, `binary`, `range`, `io`) are reported as JSON on stdout.
- CLI misuse exits `2`; unrecoverable infrastructure failures exit `1`; normal logical outcomes exit `0`.

## Project docs

- [`PRD.md`](./PRD.md) — product requirements
- [`SPEC.md`](./SPEC.md) — implementation contract
