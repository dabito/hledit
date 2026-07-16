package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	findDefaultLimit    = 500
	findDefaultMaxFiles = 100
	findMaxBytes        = 50 * 1024
	findMaxFileBytes    = 2 * 1024 * 1024
	findMaxLineBytes    = 1000
)

var findDefaultIgnoreDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"build":        true,
	"coverage":     true,
	".cache":       true,
	".next":        true,
	".nuxt":        true,
	"target":       true,
	".venv":        true,
	"__pycache__":  true,
	"vendor":       true,
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

type findOptions struct {
	Pattern  string
	Root     string
	Context  int
	Limit    int
	MaxFiles int
	Includes []string
	Excludes []string
	JSON     bool
	Pretty   bool
}

type findFileCandidate struct {
	abs string
	rel string
}

func validateFindOptions(opts findOptions) error {
	if opts.Pattern == "" {
		return fmt.Errorf("find pattern must not be empty")
	}
	if opts.Context < 0 {
		return fmt.Errorf("find --context must be >= 0")
	}
	if opts.Limit < 1 {
		return fmt.Errorf("find --limit must be >= 1")
	}
	if opts.MaxFiles < 1 {
		return fmt.Errorf("find --max-files must be >= 1")
	}
	for _, glob := range append(append([]string{}, opts.Includes...), opts.Excludes...) {
		if err := validateFindGlob(glob); err != nil {
			return err
		}
	}
	return nil
}

func validateFindGlob(glob string) error {
	if glob == "" {
		return fmt.Errorf("find glob must not be empty")
	}
	_, err := path.Match(glob, "x")
	if err == nil {
		return nil
	}
	return fmt.Errorf("bad glob %q: %w", glob, err)
}

func cmdFind(opts findOptions) error {
	if err := validateFindOptions(opts); err != nil {
		return err
	}

	result, err := collectFindResult(opts)
	if err != nil {
		return err
	}
	if opts.JSON {
		return emitJSON(result)
	}
	return emitFindText(result, opts.Pretty)
}

func collectFindResult(opts findOptions) (FindResult, error) {
	result := FindResult{OK: true, Mode: "substring", Matches: []FindMatch{}}
	root := opts.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return result, err
	}

	candidates, skipped, err := findCandidates(absRoot, opts.Includes, opts.Excludes)
	result.FilesSkipped += skipped
	if err != nil {
		return result, err
	}

	bytesEmitted := 0
	for _, candidate := range candidates {
		if result.Truncated {
			break
		}
		lines, binary, err := readFindFileLines(candidate.abs)
		if err != nil || binary {
			result.FilesSkipped++
			continue
		}
		result.FilesSearched++

		matchIdxs := filterLines(lines, opts.Pattern)
		if len(matchIdxs) == 0 {
			continue
		}
		result.MatchCount += len(matchIdxs)
		emitIdxs := applyContext(lines, matchIdxs, opts.Context)
		fileMatch := FindMatch{File: candidate.rel, Lines: []ReadLine{}}
		for _, ln := range emitIdxs {
			if result.EmittedLineCount >= opts.Limit || bytesEmitted >= findMaxBytes {
				result.Truncated = true
				break
			}
			line := lines[ln-1]
			text, textTruncated := truncateFindLine(line)
			readLine := ReadLine{Line: ln, Anchor: formatTag(ln, line), Text: text, TextTruncated: textTruncated}
			fileMatch.Lines = append(fileMatch.Lines, readLine)
			result.EmittedLineCount++
			bytesEmitted += len(candidate.rel) + len(readLine.Anchor) + len(readLine.Text) + 8
		}
		if len(fileMatch.Lines) > 0 {
			if len(result.Matches) >= opts.MaxFiles {
				result.Truncated = true
				break
			}
			result.Matches = append(result.Matches, fileMatch)
		}
	}
	return result, nil
}

func findCandidates(absRoot string, includes, excludes []string) ([]findFileCandidate, int, error) {
	candidates := []findFileCandidate{}
	skipped := 0
	seen := map[string]bool{}

	info, err := os.Lstat(absRoot)
	if err != nil {
		return nil, 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return candidates, 1, nil
	}
	if !info.IsDir() {
		rel := slashPath(filepath.Base(absRoot))
		if shouldIncludeFindPath(rel, includes, excludes) {
			candidates = append(candidates, findFileCandidate{abs: absRoot, rel: rel})
		} else {
			skipped++
		}
		return candidates, skipped, nil
	}

	err = filepath.WalkDir(absRoot, func(filePath string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == absRoot {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			skipped++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			rel, err := filepath.Rel(absRoot, filePath)
			if err != nil {
				return err
			}
			rel = slashPath(rel)
			if findDefaultIgnoreDirs[d.Name()] || matchesAnyFindGlob(excludes, rel) {
				skipped++
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			skipped++
			return nil
		}
		rel, err := filepath.Rel(absRoot, filePath)
		if err != nil {
			return err
		}
		rel = slashPath(rel)
		if !shouldIncludeFindPath(rel, includes, excludes) {
			skipped++
			return nil
		}
		realPath, err := filepath.EvalSymlinks(filePath)
		if err == nil {
			realPath = slashPath(realPath)
			if seen[realPath] {
				skipped++
				return nil
			}
			seen[realPath] = true
		}
		candidates = append(candidates, findFileCandidate{abs: filePath, rel: rel})
		return nil
	})
	if err != nil {
		return nil, skipped, err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].rel < candidates[j].rel })
	return candidates, skipped, nil
}

func shouldIncludeFindPath(rel string, includes, excludes []string) bool {
	for _, glob := range excludes {
		if matchFindGlob(glob, rel) {
			return false
		}
	}
	if len(includes) == 0 {
		return true
	}
	for _, glob := range includes {
		if matchFindGlob(glob, rel) {
			return true
		}
	}
	return false
}

func matchesAnyFindGlob(globs []string, rel string) bool {
	for _, glob := range globs {
		if matchFindGlob(glob, rel) {
			return true
		}
	}
	return false
}
func matchFindGlob(glob, rel string) bool {
	rel = slashPath(rel)
	if ok, _ := path.Match(glob, rel); ok {
		return true
	}
	if !strings.Contains(glob, "/") {
		if ok, _ := path.Match(glob, path.Base(rel)); ok {
			return true
		}
	}
	return false
}

func readFindFileLines(filePath string) ([]string, bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, false, err
	}
	if info.Size() > findMaxFileBytes {
		return nil, true, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false, err
	}
	searchLimit := len(data)
	if searchLimit > 8192 {
		searchLimit = 8192
	}
	if bytes.IndexByte(data[:searchLimit], 0x00) >= 0 {
		return nil, true, nil
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, false, nil
}

func truncateFindLine(line string) (string, bool) {
	if len(line) <= findMaxLineBytes {
		return line, false
	}
	end := findMaxLineBytes
	for end > 0 && !utf8.RuneStart(line[end]) {
		end--
	}
	if end <= 0 {
		end = findMaxLineBytes
	}
	return line[:end] + "…", true
}

func emitFindText(result FindResult, pretty bool) error {
	usePretty := prettyEnabled(pretty)
	var buf bytes.Buffer
	for i, match := range result.Matches {
		if i > 0 {
			buf.WriteString("\n")
		}
		fileHeader := match.File
		if usePretty {
			fileHeader = ansiWrap(ansiBoldCyan, fileHeader)
		}
		buf.WriteString(fileHeader + "\n")
		lineNumWidth := 1
		if len(match.Lines) > 0 {
			lineNumWidth = prettyLineNumberWidth(match.Lines[len(match.Lines)-1].Line)
		}
		for _, line := range match.Lines {
			lineStr := formatFindPlainReadLine(line.Anchor, line.Text)
			if usePretty {
				lineStr = formatFindPrettyReadLine(line.Line, line.Anchor, line.Text, lineNumWidth)
			}
			buf.WriteString(lineStr + "\n")
		}
	}
	if result.Truncated {
		notice := "-- find truncated; narrow the pattern, add --include, or raise --limit/--max-files --"
		if usePretty {
			notice = formatPrettyNotice(notice)
		}
		buf.WriteString(notice + "\n")
	}
	_, err := os.Stdout.Write(buf.Bytes())
	return err
}

func formatFindPlainReadLine(anchor, text string) string {
	return anchor + ":" + text
}

func formatFindPrettyReadLine(lineNum int, anchor, text string, lineNumWidth int) string {
	parts := strings.SplitN(anchor, "#", 2)
	if len(parts) != 2 {
		return anchor + ":" + text
	}
	linePart := padPrettyLineNumber(lineNum, lineNumWidth)
	return ansiWrap(ansiDim, linePart) + ansiWrap(ansiDim, "#") + ansiWrap(ansiBoldCyan, parts[1]) + ansiWrap(ansiDim, prettyReadSeparator) + highlightPrettyContent(text)
}

func slashPath(p string) string {
	return filepath.ToSlash(p)
}
