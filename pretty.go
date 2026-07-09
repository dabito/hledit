package main

import (
	"os"
	"strings"
)

const (
	ansiReset           = "\x1b[0m"
	ansiDim             = "\x1b[2m"
	prettyReadSeparator = "\t| "
	ansiYellow          = "\x1b[33m"
	ansiGreen           = "\x1b[32m"
	ansiBoldCyan        = "\x1b[1;36m"
)

func prettyEnabled(pretty bool) bool {
	return pretty && os.Getenv("NO_COLOR") == ""
}

func ansiWrap(code, s string) string {
	return code + s + ansiReset
}

func highlightPrettyContent(line string) string {
	var b strings.Builder
	inString := false
	escaped := false

	for _, r := range line {
		if inString {
			b.WriteRune(r)
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				b.WriteString(ansiReset)
				inString = false
			}
			continue
		}

		if r == '"' {
			b.WriteString(ansiGreen)
			b.WriteRune(r)
			inString = true
			continue
		}

		if strings.ContainsRune("{}[]()", r) {
			b.WriteString(ansiYellow)
			b.WriteRune(r)
			b.WriteString(ansiReset)
			continue
		}

		b.WriteRune(r)
	}

	if inString {
		b.WriteString(ansiReset)
	}
	return b.String()
}

func prettyLineNumberWidth(lineCount int) int {
	if lineCount < 1 {
		return 1
	}
	return len(intToStr(lineCount))
}

func padPrettyLineNumber(lineNum, width int) string {
	s := intToStr(lineNum)
	if width <= len(s) {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func formatPrettyReadLine(lineNum int, line string, lineNumWidth int) string {
	tag := formatTag(lineNum, line)
	parts := strings.SplitN(tag, "#", 2)
	if len(parts) != 2 {
		return tag + ":" + line
	}
	linePart := padPrettyLineNumber(lineNum, lineNumWidth)
	return ansiWrap(ansiDim, linePart) + ansiWrap(ansiDim, "#") + ansiWrap(ansiBoldCyan, parts[1]) + ansiWrap(ansiDim, prettyReadSeparator) + highlightPrettyContent(line)
}

func formatPrettyAnchorLine(lineNum int, line string, lineNumWidth int) string {
	tag := formatTag(lineNum, line)
	parts := strings.SplitN(tag, "#", 2)
	if len(parts) != 2 {
		return tag + "\t" + line
	}
	linePart := padPrettyLineNumber(lineNum, lineNumWidth)
	return ansiWrap(ansiDim, linePart) + ansiWrap(ansiDim, "#") + ansiWrap(ansiBoldCyan, parts[1]) + ansiWrap(ansiDim, "\t") + highlightPrettyContent(line)
}

func formatPrettyNotice(s string) string {
	return ansiWrap(ansiDim, s)
}

func formatPlainReadLine(lineNum int, line string) string {
	return formatTag(lineNum, line) + ":" + line
}

func formatPlainAnchorLine(lineNum int, line string) string {
	return formatTag(lineNum, line) + "\t" + line
}
