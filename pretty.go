package main

import (
	"os"
	"strings"
)

const (
	ansiReset    = "\x1b[0m"
	ansiDim      = "\x1b[2m"
	ansiBoldCyan = "\x1b[1;36m"
)

func prettyEnabled(pretty bool) bool {
	return pretty && os.Getenv("NO_COLOR") == ""
}

func ansiWrap(code, s string) string {
	return code + s + ansiReset
}

func formatPrettyReadLine(lineNum int, line string) string {
	tag := formatTag(lineNum, line)
	parts := strings.SplitN(tag, "#", 2)
	if len(parts) != 2 {
		return tag + ":" + line
	}
	return ansiWrap(ansiDim, parts[0]) + ansiWrap(ansiDim, "#") + ansiWrap(ansiBoldCyan, parts[1]) + ansiWrap(ansiDim, ":") + line
}

func formatPrettyAnchorLine(lineNum int, line string) string {
	tag := formatTag(lineNum, line)
	parts := strings.SplitN(tag, "#", 2)
	if len(parts) != 2 {
		return tag + "\t" + line
	}
	return ansiWrap(ansiDim, parts[0]) + ansiWrap(ansiDim, "#") + ansiWrap(ansiBoldCyan, parts[1]) + ansiWrap(ansiDim, "\t") + line
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
