package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

func readContentLines(contentSrc string) ([]string, error) {
	var raw []byte
	var err error
	if contentSrc == "-" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(contentSrc)
	}
	if err != nil {
		return nil, err
	}

	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{}, nil
	}
	return strings.Split(s, "\n"), nil
}

func contentSourceErrorMessage(contentSrc string, err error) string {
	if contentSrc == "" {
		return "content-source argument is empty; replace/replace-range/insert expect <content-source> to be '-' for stdin or a file path. To delete, pipe empty stdin and use '-' as the content-source, e.g. printf '' | hledit replace <file> <anchor> -"
	}
	return fmt.Sprintf("content-source argument %q could not be read: %v. If you intended literal replacement text, pipe it on stdin and use '-' as the content-source", contentSrc, err)
}

func targetFileErrorMessage(path string, err error) string {
	return fmt.Sprintf("file argument %q could not be read: %v", path, err)
}

func loadFileLines(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		emitError("io", targetFileErrorMessage(path, err))
		return nil, err
	}

	for i := 0; i < len(content) && i < 8192; i++ {
		if content[i] == 0x00 {
			emitError("binary", "file appears to be binary")
			return nil, fmt.Errorf("file appears to be binary")
		}
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
func emitJSONError(e EditError) error {
	return emitJSON(e)
}

func emitResult(firstChanged int) error {
	return emitJSON(EditResult{OK: true, FirstChangedLine: firstChanged})
}

func emitStaleError(remaps []Remap, msg string) error {
	return emitJSONError(EditError{OK: false, Error: "stale", Remaps: remaps, Message: msg})
}

func emitInvalidError(msg string) error {
	return emitJSONError(EditError{OK: false, Error: "invalid", Message: msg})
}

// fileHasTrailingNewline reads the raw file to check whether it ends with \n.
// This preserves the original file's trailing newline behavior after edits.
func fileHasTrailingNewline(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return len(data) > 0 && data[len(data)-1] == '\n'
}

// joinLines joins lines with \n and appends a trailing \n only if the original
// file had one (or if the file is new / doesn't exist yet).
func joinLines(lines []string, hadTrailingNewline bool) string {
	joined := strings.Join(lines, "\n")
	if hadTrailingNewline {
		joined += "\n"
	}
	return joined
}

// editOp applies a validated edit operation to the file.
// It handles: loading, anchor validation, content reading, applying the edit,
// atomic write with trailing newline preservation, and result emission.
// apply is a callback that receives the current lines and new content lines,
// and returns the resulting lines and the first changed line number.
func editOp(
	path string,
	anchors []Anchor,
	contentSrc string,
	apply func(lines []string, newLines []string) (result []string, firstChanged int, err error),
) error {
	lines, err := loadFileLines(path)
	if err != nil {
		return nil
	}

	for _, a := range anchors {
		if a.Line < 1 || a.Line > len(lines) {
			emitStaleError([]Remap{}, fmt.Sprintf("anchor %d#%s: stale", a.Line, a.Hash))
			return nil
		}
	}

	remaps, firstBad := validateAnchors(lines, anchors)
	if firstBad >= 0 {
		emitStaleError(remaps, fmt.Sprintf("anchor %d#%s: stale", anchors[firstBad].Line, anchors[firstBad].Hash))
		return nil
	}

	newLines, cerr := readContentLines(contentSrc)
	if cerr != nil {
		emitError("io", contentSourceErrorMessage(contentSrc, cerr))
		return nil
	}

	result, firstChanged, aerr := apply(lines, newLines)
	if aerr != nil {
		emitInvalidError(aerr.Error())
		return nil
	}

	hadTrailing := fileHasTrailingNewline(path)
	joined := joinLines(result, hadTrailing)
	if werr := atomicWrite(path, []byte(joined)); werr != nil {
		emitError("io", werr.Error())
		return nil
	}

	emitResult(firstChanged)
	return nil
}

func cmdReplace(path, anchorStr, contentSrc string) error {
	a, perr := parseAnchor(anchorStr)
	if perr != nil {
		emitInvalidError(perr.Error())
		return nil
	}

	return editOp(path, []Anchor{a}, contentSrc, func(lines, newLines []string) ([]string, int, error) {
		before := append([]string{}, lines[:a.Line-1]...)
		result := append(before, newLines...)
		result = append(result, lines[a.Line:]...)
		return result, a.Line, nil
	})
}

func cmdReplaceRange(path, anchorStr, endAnchorStr, contentSrc string) error {
	a, perr := parseAnchor(anchorStr)
	if perr != nil {
		emitInvalidError(perr.Error())
		return nil
	}
	e, perr2 := parseAnchor(endAnchorStr)
	if perr2 != nil {
		emitInvalidError(perr2.Error())
		return nil
	}
	if a.Line > e.Line {
		emitInvalidError(fmt.Sprintf("start line %d > end line %d", a.Line, e.Line))
		return nil
	}

	return editOp(path, []Anchor{a, e}, contentSrc, func(lines, newLines []string) ([]string, int, error) {
		before := lines[:a.Line-1]
		after := lines[e.Line:]
		result := append(append([]string{}, before...), newLines...)
		result = append(result, after...)
		return result, a.Line, nil
	})
}

func cmdInsert(path, anchorStr, contentSrc string, after bool) error {
	a, perr := parseAnchor(anchorStr)
	if perr != nil {
		emitInvalidError(perr.Error())
		return nil
	}

	return editOp(path, []Anchor{a}, contentSrc, func(lines, newLines []string) ([]string, int, error) {
		if len(newLines) == 0 {
			return nil, 0, fmt.Errorf("insert requires non-empty content")
		}
		cutIdx := a.Line - 1
		firstChanged := a.Line
		if after {
			cutIdx = a.Line
			firstChanged = a.Line + 1
		}
		result := append([]string{}, lines[:cutIdx]...)
		result = append(result, newLines...)
		result = append(result, lines[cutIdx:]...)
		return result, firstChanged, nil
	})
}

// cmdBatch applies multiple edit operations in a single pass.
// All anchors are validated against the same file state, then edits are
// applied bottom-up so earlier line numbers remain valid.
// Input is a JSON BatchEditRequest on stdin.
func cmdBatch(path string) error {
	req, err := parseBatchRequest()
	if err != nil {
		emitBatchError(fmt.Sprintf("invalid batch request: %s", err.Error()), nil, -1)
		return nil
	}

	if len(req.Edits) == 0 {
		emitBatchError("batch request contains no edits", nil, -1)
		return nil
	}

	lines, loadErr := loadFileLines(path)
	if loadErr != nil {
		return nil
	}

	// Parse all anchors first, before any edits are applied.
	type parsedEdit struct {
		op      string
		pos     Anchor
		endPos  *Anchor
		lines   []string
		lineNum int // original line number for this edit
	}

	parsed := make([]parsedEdit, len(req.Edits))
	var allRemaps []Remap
	firstBad := -1

	for i, e := range req.Edits {
		pos, perr := parseAnchor(e.Pos)
		if perr != nil {
			emitBatchError(fmt.Sprintf("edit %d: invalid anchor %q: %s", i, e.Pos, perr.Error()), nil, i)
			return nil
		}

		var endPos *Anchor
		if e.EndPos != "" {
			ep, eerr := parseAnchor(e.EndPos)
			if eerr != nil {
				emitBatchError(fmt.Sprintf("edit %d: invalid end anchor %q: %s", i, e.EndPos, eerr.Error()), nil, i)
				return nil
			}
			endPos = &ep
		}

		newLines, cerr := readContentLinesFromOps(e.Lines)
		if cerr != nil {
			emitBatchError(fmt.Sprintf("edit %d: %s", i, cerr.Error()), nil, i)
			return nil
		}

		// Validate anchor against current file state
		if pos.Line < 1 || pos.Line > len(lines) {
			if firstBad == -1 {
				firstBad = i
			}
			allRemaps = append(allRemaps, Remap{
				Requested: intToStr(pos.Line) + "#" + pos.Hash,
			})
		} else {
			currentTag := formatTag(pos.Line, lines[pos.Line-1])
			requestedTag := intToStr(pos.Line) + "#" + pos.Hash
			if currentTag != requestedTag {
				if firstBad == -1 {
					firstBad = i
				}
				allRemaps = append(allRemaps, Remap{
					Requested: requestedTag,
					Current:   currentTag,
				})
			}
		}

		if endPos != nil {
			if endPos.Line < 1 || endPos.Line > len(lines) {
				if firstBad == -1 {
					firstBad = i
				}
				allRemaps = append(allRemaps, Remap{
					Requested: intToStr(endPos.Line) + "#" + endPos.Hash,
				})
			} else {
				currentTag := formatTag(endPos.Line, lines[endPos.Line-1])
				requestedTag := intToStr(endPos.Line) + "#" + endPos.Hash
				if currentTag != requestedTag {
					if firstBad == -1 {
						firstBad = i
					}
					allRemaps = append(allRemaps, Remap{
						Requested: requestedTag,
						Current:   currentTag,
					})
				}
			}
		}

		parsed[i] = parsedEdit{
			op:      e.OP,
			pos:     pos,
			endPos:  endPos,
			lines:   newLines,
			lineNum: pos.Line,
		}
	}

	// If any anchor is stale, reject the entire batch
	if firstBad >= 0 {
		emitBatchError(
			fmt.Sprintf("edit %d: anchor stale", firstBad),
			allRemaps,
			firstBad,
		)
		return nil
	}

	// Sort edits by line number descending (bottom-up) so earlier anchors
	// remain valid after each application.
	sort.SliceStable(parsed, func(i, j int) bool {
		return parsed[i].lineNum > parsed[j].lineNum
	})

	firstChanged := parsed[0].lineNum

	// Apply edits bottom-up
	for _, e := range parsed {
		switch e.op {
		case "replace", "delete":
			eop := e.endPos
			if eop == nil {
				eop = &e.pos
			}
			before := append([]string{}, lines[:e.pos.Line-1]...)
			after := lines[eop.Line:]
			lines = append(before, e.lines...)
			lines = append(lines, after...)
		case "insert":
			cutIdx := e.pos.Line - 1
			result := append([]string{}, lines[:cutIdx]...)
			result = append(result, e.lines...)
			result = append(result, lines[cutIdx:]...)
			lines = result
		default:
			emitBatchError(fmt.Sprintf("unknown op %q", e.op), nil, -1)
			return nil
		}

		if e.lineNum < firstChanged {
			firstChanged = e.lineNum
		}
	}

	hadTrailing := fileHasTrailingNewline(path)
	joined := joinLines(lines, hadTrailing)
	if werr := atomicWrite(path, []byte(joined)); werr != nil {
		emitError("io", werr.Error())
		return nil
	}

	return emitJSON(BatchEditResult{
		OK:               true,
		FirstChangedLine: firstChanged,
		EditsApplied:     len(parsed),
	})
}

func emitBatchError(msg string, remaps []Remap, failed int) error {
	return emitJSON(BatchEditError{
		OK:      false,
		Error:   "stale",
		Message: msg,
		Remaps:  remaps,
		Failed:  failed,
	})
}

// readContentLinesFromOps reads content from inline lines (used by batch).
// Empty lines slice means delete.
func readContentLinesFromOps(lines []string) ([]string, error) {
	if len(lines) == 0 {
		return []string{}, nil
	}
	return lines, nil
}
