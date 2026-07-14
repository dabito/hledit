package main

import (
	"flag"
	"fmt"
	"os"
)

const version = "1.3.3"

// splitArgs separates a command's args into flags and positionals so that
// flags may appear before OR after the positional file argument (e.g.
// "read-range main.go --offset 4" and "read-range --offset 4 main.go" both
// work). Go's flag package stops at the first non-flag arg, so we reorder.
// A bare "-" is treated as a positional (stdin content-source).
func splitArgs(args []string) (positionals []string, flags []string) {
	// Flags that take a value ("-x v" form) in our subcommands.
	valueFlags := map[string]bool{"-offset": true, "--offset": true, "-limit": true, "--limit": true, "-grep": true, "--grep": true, "-context": true, "--context": true}
	boolFlags := map[string]bool{"--before": true, "--after": true, "--json": true, "-json": true, "--pretty": true, "-pretty": true, "--check": true, "-check": true}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" {
			positionals = append(positionals, a)
			continue
		}
		if valueFlags[a] {
			flags = append(flags, a)
			if i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		if boolFlags[a] {
			flags = append(flags, a)
			continue
		}
		if len(a) > 0 && a[0] == '-' {
			positionals = append(positionals, a)
			continue
		}
		positionals = append(positionals, a)
	}
	return positionals, flags
}

const usage = `hledit — hash-anchored line editor for AI coding agents

Usage:
  hledit --version
  hledit help [command]
  hledit read <file> [--grep <pattern>] [--context N] [--json] [--pretty]
  hledit read-range <file> [--offset N] [--limit M] [--grep <pattern>] [--context N] [--json] [--pretty]
  hledit anchors <file> [--offset N] [--limit M] [--grep <pattern>] [--context N] [--json] [--pretty]
  hledit replace <file> <anchor> <content-source>
  hledit replace-range <file> <anchor> <end-anchor> <content-source>
  hledit insert [--before|--after] <file> <anchor> <content-source>
  hledit batch [--check] <file>

Arguments:
  <anchor>          LN#HASH from a prior read, e.g. 5#ABC (legacy 5#WS accepted)
  <content-source>  - for stdin, or a file path

Batch input (JSON on stdin):
  {"edits": [
    {"op": "replace", "pos": "12#NKA", "lines": ["new line"]},
    {"op": "replace", "pos": "12#NKA", "end_pos": "18#VRC", "lines": ["new block"]},
    {"op": "delete", "pos": "5#TXA", "lines": []},
    {"op": "insert", "pos": "8#VRB", "lines": ["inserted"]}
  ]}

Examples:
  hledit read main.go
  hledit read-range main.go --offset 40 --limit 20
  hledit read --help
  printf '  return nil\n' | hledit replace main.go 12#NKA -
  hledit replace-range main.go 12#NKA 18#VRC /tmp/new-block.txt
  cat header.txt | hledit insert --before main.go 1#QVE -
  printf '// done\n' | hledit insert --after main.go 99#TXA -
  echo '{"edits":[{"op":"replace","pos":"12#NKA","lines":["fixed"]}]}' | hledit batch main.go
  echo '{"edits":[{"op":"replace","pos":"12#NKA","lines":["fixed"]}]}' | hledit batch --check main.go

Notes:
  - Use "hledit <command> --help" or "hledit help <command>" for command-specific help.
  - replace/replace-range with empty content deletes the target line/range.
  - batch applies multiple edits atomically: all anchors validated first,
    then edits applied bottom-up, then a single atomic write.
  - batch --check validates all anchors and ops without writing; result includes checked:true.
  - All write verbs validate anchors before writing. If any anchor is stale,
    nothing is written and stdout contains JSON {"ok":false,"error":"stale",...}.
  - Logical errors exit 0 and are reported as JSON on stdout; CLI misuse exits 2.
`

const readUsage = `hledit read — print anchored lines from a whole file

Usage:
  hledit read <file> [--grep <pattern>] [--context N] [--json] [--pretty]

Flags:
  --grep <pattern>  filter lines by substring match
  --context N       include N surrounding lines for each grep match
  --json            emit structured JSON instead of annotated text
  --pretty          emit ANSI-styled text for human reading
`

const readRangeUsage = `hledit read-range — print anchored lines from a bounded line range

Usage:
  hledit read-range <file> [--offset N] [--limit M] [--grep <pattern>] [--context N] [--json] [--pretty]

Flags:
  --offset N        1-indexed starting line (default 1)
  --limit M         max lines to return (default 2000)
  --grep <pattern>  filter lines by substring match
  --context N       include N surrounding lines for each grep match
  --json            emit structured JSON instead of annotated text
  --pretty          emit ANSI-styled text for human reading
`

const anchorsUsage = `hledit anchors — print anchors and text from a bounded line range

Usage:
  hledit anchors <file> [--offset N] [--limit M] [--grep <pattern>] [--context N] [--json] [--pretty]

Flags:
  --offset N        1-indexed starting line (default 1)
  --limit M         max lines to return (default 2000)
  --grep <pattern>  filter lines by substring match
  --context N       include N surrounding lines for each grep match
  --json            emit structured JSON instead of tab-separated text
  --pretty          emit ANSI-styled text for human reading
`

const replaceUsage = `hledit replace — replace one anchored line

Usage:
  hledit replace <file> <anchor> <content-source>

Arguments:
  <anchor>          LN#HASH from a prior read, e.g. 5#ABC
  <content-source>  - for stdin, or a file path

Notes:
  Empty content deletes the target line.
`

const replaceRangeUsage = `hledit replace-range — replace an anchored line range

Usage:
  hledit replace-range <file> <anchor> <end-anchor> <content-source>

Arguments:
  <anchor>          first LN#HASH from a prior read
  <end-anchor>      last LN#HASH from a prior read
  <content-source>  - for stdin, or a file path

Notes:
  Empty content deletes the target range.
`

const insertUsage = `hledit insert — insert lines before or after an anchor

Usage:
  hledit insert [--before|--after] <file> <anchor> <content-source>

Flags:
  --before          insert before the anchor (default)
  --after           insert after the anchor

Arguments:
  <anchor>          LN#HASH from a prior read
  <content-source>  - for stdin, or a file path
`

const batchUsage = `hledit batch — apply multiple anchored edits atomically

Usage:
  hledit batch [--check] <file>

Flags:
  --check           validate only, do not write

Input:
  JSON on stdin with an edits array. All anchors validate before any write.
`

const versionUsage = `hledit version — print version

Usage:
  hledit version
  hledit --version
`

func commandUsage(verb string) (string, bool) {
	switch verb {
	case "read":
		return readUsage, true
	case "read-range":
		return readRangeUsage, true
	case "anchors":
		return anchorsUsage, true
	case "replace":
		return replaceUsage, true
	case "replace-range":
		return replaceRangeUsage, true
	case "insert":
		return insertUsage, true
	case "batch":
		return batchUsage, true
	case "version":
		return versionUsage, true
	default:
		return "", false
	}
}

func isHelpArg(arg string) bool {
	return arg == "help" || arg == "-h" || arg == "--help"
}

func isCommandHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--help"
}

func containsCommandHelpFlag(args []string) bool {
	for _, arg := range args {
		if isCommandHelpFlag(arg) {
			return true
		}
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(argv []string) int {
	if len(argv) < 1 {
		fmt.Print(usage)
		return 0
	}

	// Handle --version globally
	if argv[0] == "--version" || argv[0] == "-v" {
		fmt.Printf("hledit %s\n", version)
		return 0
	}

	if isHelpArg(argv[0]) {
		if len(argv) == 1 {
			fmt.Print(usage)
			return 0
		}
		if len(argv) == 2 {
			if help, ok := commandUsage(argv[1]); ok {
				fmt.Print(help)
				return 0
			}
			fmt.Fprintf(os.Stderr, "unknown help topic %q\n\n%s", argv[1], usage)
			return 2
		}
		fmt.Fprintf(os.Stderr, "too many help topics\n\n%s", usage)
		return 2
	}

	verb := argv[0]
	args := argv[1:]
	if containsCommandHelpFlag(args) {
		if help, ok := commandUsage(verb); ok {
			fmt.Print(help)
			return 0
		}
		fmt.Fprintf(os.Stderr, "unknown verb %q\n\n%s", verb, usage)
		return 2
	}
	switch verb {
	case "read":
		positionals, flagArgs := splitArgs(args)
		fs := flag.NewFlagSet("read", flag.ExitOnError)
		grep := fs.String("grep", "", "filter lines by substring match")
		contextN := fs.Int("context", 0, "include N surrounding lines for each grep match")
		pretty := fs.Bool("pretty", false, "emit ANSI-styled text for human reading")
		jsonOut := fs.Bool("json", false, "emit structured JSON instead of annotated text")
		fs.Parse(flagArgs)
		if len(positionals) != 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdReadPretty(positionals[0], *grep, *contextN, *jsonOut, *pretty))

	case "read-range":
		positionals, flagArgs := splitArgs(args)
		fs := flag.NewFlagSet("read-range", flag.ExitOnError)
		offset := fs.Int("offset", 1, "1-indexed starting line")
		limit := fs.Int("limit", 2000, "max lines to return")
		grep := fs.String("grep", "", "filter lines by substring match")
		contextN := fs.Int("context", 0, "include N surrounding lines for each grep match")
		pretty := fs.Bool("pretty", false, "emit ANSI-styled text for human reading")
		jsonOut := fs.Bool("json", false, "emit structured JSON instead of annotated text")
		fs.Parse(flagArgs)
		if len(positionals) != 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdReadRangePretty(positionals[0], *offset, *limit, *grep, *contextN, *jsonOut, *pretty))

	case "anchors":
		positionals, flagArgs := splitArgs(args)
		fs := flag.NewFlagSet("anchors", flag.ExitOnError)
		offset := fs.Int("offset", 1, "1-indexed starting line")
		limit := fs.Int("limit", 2000, "max lines to return")
		grep := fs.String("grep", "", "filter lines by substring match")
		contextN := fs.Int("context", 0, "include N surrounding lines for each grep match")
		pretty := fs.Bool("pretty", false, "emit ANSI-styled text for human reading")
		jsonOut := fs.Bool("json", false, "emit structured JSON instead of annotated text")
		fs.Parse(flagArgs)
		if len(positionals) != 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdAnchorsPretty(positionals[0], *offset, *limit, *grep, *contextN, *jsonOut, *pretty))

	case "replace":
		if len(args) != 3 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdReplace(args[0], args[1], args[2]))

	case "replace-range":
		if len(args) != 4 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdReplaceRange(args[0], args[1], args[2], args[3]))

	case "insert":
		positionals, flagArgs := splitArgs(args)
		fs := flag.NewFlagSet("insert", flag.ExitOnError)
		before := fs.Bool("before", false, "insert before the anchor (default)")
		after := fs.Bool("after", false, "insert after the anchor")
		fs.Parse(flagArgs)
		if len(positionals) != 3 || (*before && *after) {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdInsert(positionals[0], positionals[1], positionals[2], *after))

	case "batch":
		positionals, flagArgs := splitArgs(args)
		fs := flag.NewFlagSet("batch", flag.ExitOnError)
		check := fs.Bool("check", false, "validate only, do not write")
		fs.Parse(flagArgs)
		if len(positionals) != 1 {
			fmt.Fprint(os.Stderr, usage)
			return 2
		}
		return mustRun(cmdBatch(positionals[0], *check))

	case "version":
		fmt.Printf("hledit %s\n", version)
		return 0

	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0

	default:
		fmt.Fprintf(os.Stderr, "unknown verb %q\n\n%s", verb, usage)
		return 2
	}
}

// mustRun handles the return value of a cmd* function. Per SPEC §9, cmd*
// functions return nil for all logical errors (they emit JSON themselves);
// a non-nil return indicates an unrecoverable infrastructure failure → exit 1.
func mustRun(err error) int {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
