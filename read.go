package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

func emitError(errType, message string) error {
	return emitJSON(EditError{
		OK:      false,
		Error:   errType,
		Message: message,
	})
}

// readFileLines reads a file, checks for binary content, and returns its lines.
// Returns nil lines and true if an error was already emitted to stdout.
func readFileLines(path string) ([]string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		emitError("io", err.Error())
		return nil, true
	}

	// Binary detection (first 8 KB)
	searchLimit := len(data)
	if searchLimit > 8192 {
		searchLimit = 8192
	}
	for i := 0; i < searchLimit; i++ {
		if data[i] == 0x00 {
			emitError("binary", "file appears to be binary")
			return nil, true
		}
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, false
}

// filterLines returns 1-indexed line numbers of lines matching the pattern.
// If pattern is empty, nil is returned (meaning no filtering).
func filterLines(lines []string, pattern string) []int {
	if pattern == "" {
		return nil
	}
	matches := make([]int, 0)
	for i, line := range lines {
		if strings.Contains(line, pattern) {
			matches = append(matches, i+1) // 1-indexed
		}
	}
	return matches
}

// applyContext expands matchIdxs by including up to contextN lines before and
// after each match. Overlapping windows are merged. Returns a sorted,
// deduplicated slice of 1-indexed line numbers. If contextN <= 0 or matchIdxs
// is empty the original slice is returned unchanged.
func applyContext(lines []string, matchIdxs []int, contextN int) []int {
	if contextN <= 0 || len(matchIdxs) == 0 {
		return matchIdxs
	}
	total := len(lines)
	included := make([]bool, total+1) // 1-indexed; index 0 unused
	for _, ln := range matchIdxs {
		start := ln - contextN
		if start < 1 {
			start = 1
		}
		end := ln + contextN
		if end > total {
			end = total
		}
		for i := start; i <= end; i++ {
			included[i] = true
		}
	}
	result := make([]int, 0, len(matchIdxs))
	for i := 1; i <= total; i++ {
		if included[i] {
			result = append(result, i)
		}
	}
	return result
}

// emitAnnotatedLines writes LN#HASH:content lines to a buffer with truncation.
// Returns the number of content lines emitted.
func emitAnnotatedLines(buf *bytes.Buffer, lines []string, startIdx, maxLines, maxBytes int, pretty bool) int {
	emittedCount := 0
	usePretty := prettyEnabled(pretty)
	lineNumWidth := prettyLineNumberWidth(len(lines))
	for i := startIdx; i < len(lines); i++ {
		lineNum := i + 1
		line := lines[i]
		lineStr := formatPlainReadLine(lineNum, line) + "\n"
		if usePretty {
			lineStr = formatPrettyReadLine(lineNum, line, lineNumWidth) + "\n"
		}

		buf.WriteString(lineStr)
		emittedCount++

		if emittedCount >= maxLines || buf.Len() >= maxBytes {
			if i < len(lines)-1 {
				notice := fmt.Sprintf("-- truncated: use read-range --offset %d --", i+2)
				if usePretty {
					notice = formatPrettyNotice(notice)
				}
				buf.WriteString(notice + "\n")
			}
			break
		}
	}
	return emittedCount
}

// collectAnnotatedLines gathers lines into ReadLine structs with truncation metadata.
// Mirrors emitAnnotatedLines byte/line limits exactly.
func collectAnnotatedLines(lines []string, startIdx, maxLines, maxBytes int) ([]ReadLine, bool, int) {
	result := make([]ReadLine, 0)
	byteCount := 0
	for i := startIdx; i < len(lines); i++ {
		lineNum := i + 1
		line := lines[i]
		tag := formatTag(lineNum, line)
		byteCount += len(tag) + 1 + len(line) + 1 // tag + ":" + line + "\n"
		result = append(result, ReadLine{Line: lineNum, Anchor: tag, Text: line})
		if len(result) >= maxLines || byteCount >= maxBytes {
			if i < len(lines)-1 {
				return result, true, i + 2
			}
			break
		}
	}
	return result, false, 0
}

// collectMatchLines gathers matching lines into ReadLine structs with truncation metadata.
// matchIdxs are 1-indexed line numbers into lines.
func collectMatchLines(lines []string, matchIdxs []int, offset, maxLines int) ([]ReadLine, bool, int) {
	startIdx := len(matchIdxs)
	for i, ln := range matchIdxs {
		if ln >= offset {
			startIdx = i
			break
		}
	}
	result := make([]ReadLine, 0)
	for i := startIdx; i < len(matchIdxs) && len(result) < maxLines; i++ {
		ln := matchIdxs[i]
		line := lines[ln-1]
		tag := formatTag(ln, line)
		result = append(result, ReadLine{Line: ln, Anchor: tag, Text: line})
	}
	remaining := len(matchIdxs) - startIdx - len(result)
	if remaining > 0 {
		lastLn := matchIdxs[startIdx+len(result)-1]
		return result, true, lastLn + 1
	}
	return result, false, 0
}

// emitMatchLines writes only matching LN#HASH:content lines with pagination info.
// matchIdxs are 1-indexed line numbers into lines.
func emitMatchLines(buf *bytes.Buffer, lines []string, matchIdxs []int, offset, maxLines int, pretty bool) {
	startIdx := len(matchIdxs)
	for i, ln := range matchIdxs {
		if ln >= offset {
			startIdx = i
			break
		}
	}

	usePretty := prettyEnabled(pretty)
	lineNumWidth := prettyLineNumberWidth(len(lines))
	count := 0
	for i := startIdx; i < len(matchIdxs) && count < maxLines; i++ {
		ln := matchIdxs[i]
		line := lines[ln-1]
		lineStr := formatPlainReadLine(ln, line)
		if usePretty {
			lineStr = formatPrettyReadLine(ln, line, lineNumWidth)
		}
		buf.WriteString(lineStr + "\n")
		count++
	}

	remaining := len(matchIdxs) - startIdx - count
	if remaining > 0 {
		lastLn := matchIdxs[startIdx+count-1]
		notice := fmt.Sprintf("-- %d more matches, use offset %d --", remaining, lastLn+1)
		if usePretty {
			notice = formatPrettyNotice(notice)
		}
		buf.WriteString(notice + "\n")
	}
}

func cmdRead(path, grep string, contextN int, jsonOut bool) error {
	return cmdReadPretty(path, grep, contextN, jsonOut, false)
}

func cmdReadPretty(path, grep string, contextN int, jsonOut bool, pretty bool) error {
	lines, errored := readFileLines(path)
	if errored {
		return nil
	}

	matchIdxs := filterLines(lines, grep)

	if jsonOut {
		var readLines []ReadLine
		var truncated bool
		var nextOffset int
		if matchIdxs != nil {
			matchIdxs = applyContext(lines, matchIdxs, contextN)
			readLines, truncated, nextOffset = collectMatchLines(lines, matchIdxs, 1, 2000)
		} else {
			readLines, truncated, nextOffset = collectAnnotatedLines(lines, 0, 2000, 50*1024)
		}
		return emitJSON(ReadResult{OK: true, Lines: readLines, Truncated: truncated, NextOffset: nextOffset})
	}

	var buf bytes.Buffer
	if matchIdxs != nil {
		matchIdxs = applyContext(lines, matchIdxs, contextN)
		emitMatchLines(&buf, lines, matchIdxs, 1, 2000, pretty)
	} else {
		emitAnnotatedLines(&buf, lines, 0, 2000, 50*1024, pretty)
	}
	fmt.Print(buf.String())
	return nil
}

// emitAnchorLines writes ANCHOR\tTEXT lines (completion-friendly) with truncation.
func emitAnchorLines(buf *bytes.Buffer, lines []string, startIdx, maxLines, maxBytes int, pretty bool) {
	emittedCount := 0
	usePretty := prettyEnabled(pretty)
	lineNumWidth := prettyLineNumberWidth(len(lines))
	for i := startIdx; i < len(lines); i++ {
		lineNum := i + 1
		line := lines[i]
		lineStr := formatPlainAnchorLine(lineNum, line) + "\n"
		if usePretty {
			lineStr = formatPrettyAnchorLine(lineNum, line, lineNumWidth) + "\n"
		}

		buf.WriteString(lineStr)
		emittedCount++

		if emittedCount >= maxLines || buf.Len() >= maxBytes {
			if i < len(lines)-1 {
				notice := fmt.Sprintf("-- truncated: use anchors --offset %d --", i+2)
				if usePretty {
					notice = formatPrettyNotice(notice)
				}
				buf.WriteString(notice + "\n")
			}
			break
		}
	}
}

// emitAnchorMatchLines writes matching ANCHOR\tTEXT lines with pagination notice.
func emitAnchorMatchLines(buf *bytes.Buffer, lines []string, matchIdxs []int, offset, maxLines int, pretty bool) {
	startIdx := len(matchIdxs)
	for i, ln := range matchIdxs {
		if ln >= offset {
			startIdx = i
			break
		}
	}

	usePretty := prettyEnabled(pretty)
	lineNumWidth := prettyLineNumberWidth(len(lines))
	count := 0
	for i := startIdx; i < len(matchIdxs) && count < maxLines; i++ {
		ln := matchIdxs[i]
		line := lines[ln-1]
		lineStr := formatPlainAnchorLine(ln, line)
		if usePretty {
			lineStr = formatPrettyAnchorLine(ln, line, lineNumWidth)
		}
		buf.WriteString(lineStr + "\n")
		count++
	}

	remaining := len(matchIdxs) - startIdx - count
	if remaining > 0 {
		lastLn := matchIdxs[startIdx+count-1]
		notice := fmt.Sprintf("-- %d more matches, use offset %d --", remaining, lastLn+1)
		if usePretty {
			notice = formatPrettyNotice(notice)
		}
		buf.WriteString(notice + "\n")
	}
}

func cmdAnchors(path string, offset, limit int, grep string, contextN int, jsonOut bool) error {
	return cmdAnchorsPretty(path, offset, limit, grep, contextN, jsonOut, false)
}

func cmdAnchorsPretty(path string, offset, limit int, grep string, contextN int, jsonOut bool, pretty bool) error {
	lines, errored := readFileLines(path)
	if errored {
		return nil
	}

	if offset < 1 {
		offset = 1
	}
	if offset > len(lines) {
		emitError("range", fmt.Sprintf("offset %d exceeds file length %d", offset, len(lines)))
		return nil
	}

	maxLines := limit
	if maxLines <= 0 {
		maxLines = 2000
	}

	matchIdxs := filterLines(lines, grep)

	if jsonOut {
		var readLines []ReadLine
		var truncated bool
		var nextOffset int
		if matchIdxs != nil {
			matchIdxs = applyContext(lines, matchIdxs, contextN)
			readLines, truncated, nextOffset = collectMatchLines(lines, matchIdxs, offset, maxLines)
		} else {
			readLines, truncated, nextOffset = collectAnnotatedLines(lines, offset-1, maxLines, 50*1024)
		}
		return emitJSON(ReadResult{OK: true, Lines: readLines, Truncated: truncated, NextOffset: nextOffset})
	}

	var buf bytes.Buffer
	if matchIdxs != nil {
		matchIdxs = applyContext(lines, matchIdxs, contextN)
		emitAnchorMatchLines(&buf, lines, matchIdxs, offset, maxLines, pretty)
	} else {
		emitAnchorLines(&buf, lines, offset-1, maxLines, 50*1024, pretty)
	}

	fmt.Print(buf.String())
	return nil
}

func cmdReadRange(path string, offset, limit int, grep string, contextN int, jsonOut bool) error {
	return cmdReadRangePretty(path, offset, limit, grep, contextN, jsonOut, false)
}

func cmdReadRangePretty(path string, offset, limit int, grep string, contextN int, jsonOut bool, pretty bool) error {
	lines, errored := readFileLines(path)
	if errored {
		return nil
	}

	if offset < 1 {
		offset = 1
	}
	if offset > len(lines) {
		emitError("range", fmt.Sprintf("offset %d exceeds file length %d", offset, len(lines)))
		return nil
	}

	maxLines := limit
	if maxLines <= 0 {
		maxLines = 2000
	}

	matchIdxs := filterLines(lines, grep)

	if jsonOut {
		var readLines []ReadLine
		var truncated bool
		var nextOffset int
		if matchIdxs != nil {
			matchIdxs = applyContext(lines, matchIdxs, contextN)
			readLines, truncated, nextOffset = collectMatchLines(lines, matchIdxs, offset, maxLines)
		} else {
			readLines, truncated, nextOffset = collectAnnotatedLines(lines, offset-1, maxLines, 50*1024)
		}
		return emitJSON(ReadResult{OK: true, Lines: readLines, Truncated: truncated, NextOffset: nextOffset})
	}

	var buf bytes.Buffer
	if matchIdxs != nil {
		matchIdxs = applyContext(lines, matchIdxs, contextN)
		emitMatchLines(&buf, lines, matchIdxs, offset, maxLines, pretty)
	} else {
		emitAnnotatedLines(&buf, lines, offset-1, maxLines, 50*1024, pretty)
	}

	fmt.Print(buf.String())
	return nil
}
