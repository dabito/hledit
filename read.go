package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func emitError(errType, message string) error {
	errObj := EditError{
		OK:      false,
		Error:   errType,
		Message: message,
	}
	output, err := json.Marshal(errObj)
	if err != nil {
		return err
	}
	_, err = fmt.Println(string(output))
	return err
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

// emitAnnotatedLines writes LN#HASH:content lines to a buffer with truncation.
// Returns the number of content lines emitted.
func emitAnnotatedLines(buf *bytes.Buffer, lines []string, startIdx, maxLines, maxBytes int) int {
	emittedCount := 0
	for i := startIdx; i < len(lines); i++ {
		lineNum := i + 1
		line := lines[i]
		tag := formatTag(lineNum, line)
		lineStr := tag + ":" + line + "\n"

		buf.WriteString(lineStr)
		emittedCount++

		if emittedCount >= maxLines || buf.Len() >= maxBytes {
			if i < len(lines)-1 {
				buf.WriteString(fmt.Sprintf("-- truncated: use read-range --offset %d --\n", i+2))
			}
			break
		}
	}
	return emittedCount
}

// emitMatchLines writes only matching LN#HASH:content lines with pagination info.
// matchIdxs are 1-indexed line numbers into lines.
func emitMatchLines(buf *bytes.Buffer, lines []string, matchIdxs []int, offset, maxLines int) {
	startIdx := 0
	for i, ln := range matchIdxs {
		if ln >= offset {
			startIdx = i
			break
		}
	}

	count := 0
	for i := startIdx; i < len(matchIdxs) && count < maxLines; i++ {
		ln := matchIdxs[i]
		line := lines[ln-1]
		tag := formatTag(ln, line)
		buf.WriteString(tag + ":" + line + "\n")
		count++
	}

	remaining := len(matchIdxs) - startIdx - count
	if remaining > 0 {
		lastLn := matchIdxs[startIdx+count-1]
		buf.WriteString(fmt.Sprintf("-- %d more matches, use offset %d --\n", remaining, lastLn+1))
	}
}

func cmdRead(path string) error {
	lines, errored := readFileLines(path)
	if errored {
		return nil
	}

	var buf bytes.Buffer
	emitAnnotatedLines(&buf, lines, 0, 2000, 50*1024)
	fmt.Print(buf.String())
	return nil
}

func cmdReadRange(path string, offset, limit int, grep string) error {
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

	var buf bytes.Buffer

	matchIdxs := filterLines(lines, grep)
	if matchIdxs != nil {
		// Build annotated lines only for matches
		emitMatchLines(&buf, lines, matchIdxs, offset, maxLines)
	} else {
		emitAnnotatedLines(&buf, lines, offset-1, maxLines, 50*1024)
	}

	fmt.Print(buf.String())
	return nil
}
