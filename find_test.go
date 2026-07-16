package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func findTestWriteFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func findTestRun(t *testing.T, opts findOptions) string {
	t.Helper()
	return readTestCaptureStdout(t, func() {
		if err := cmdFind(opts); err != nil {
			t.Fatalf("cmdFind returned error: %v", err)
		}
	})
}

func TestCmdFindTextGroupsAnchorsByFile(t *testing.T) {
	dir := t.TempDir()
	findTestWriteFile(t, dir, "b.txt", "skip\nneedle b\n")
	findTestWriteFile(t, dir, "a.txt", "before\nneedle a\nafter\n")

	out := findTestRun(t, findOptions{Pattern: "needle", Root: dir, Limit: 10, MaxFiles: 10})
	lines := readTestLines(t, out)
	if len(lines) != 5 {
		t.Fatalf("find output lines = %d, want 5:\n%s", len(lines), out)
	}
	if lines[0] != "a.txt" || lines[3] != "b.txt" {
		t.Fatalf("file grouping/order unexpected:\n%s", out)
	}
	if lines[1] != formatTag(2, "needle a")+":needle a" || lines[4] != formatTag(2, "needle b")+":needle b" {
		t.Fatalf("anchored lines unexpected:\n%s", out)
	}
	if lines[2] != "" {
		t.Fatalf("want blank separator, got %q", lines[2])
	}
}

func TestCmdFindJSONPreservesFileAnchorPairingAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := findTestWriteFile(t, dir, "src/auth.txt", "header\nvalidateToken\nfooter\n")
	findTestWriteFile(t, dir, "other/auth.txt", "header\nvalidateToken\nfooter\n")

	out := findTestRun(t, findOptions{Pattern: "validateToken", Root: dir, Limit: 10, MaxFiles: 10, JSON: true})
	var got FindResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, out)
	}
	if !got.OK || got.Mode != "substring" || got.Truncated || got.MatchCount != 2 || got.EmittedLineCount != 2 {
		t.Fatalf("find result metadata unexpected: %+v", got)
	}
	if len(got.Matches) != 2 || got.Matches[0].File != "other/auth.txt" || got.Matches[1].File != "src/auth.txt" {
		t.Fatalf("file groups/order unexpected: %+v", got.Matches)
	}
	if got.Matches[0].Lines[0].Anchor != got.Matches[1].Lines[0].Anchor {
		t.Fatalf("same line/text should produce same file-agnostic anchor")
	}

	repl := filepath.Join(dir, "replacement.txt")
	if err := os.WriteFile(repl, []byte("validateTokenFixed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	anchor := got.Matches[1].Lines[0].Anchor
	if err := cmdReplace(path, anchor, repl); err != nil {
		t.Fatalf("cmdReplace returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "validateTokenFixed") {
		t.Fatalf("replace did not use find anchor, content=%q", string(data))
	}
}

func TestCmdFindContextLimitAndLineTruncation(t *testing.T) {
	dir := t.TempDir()
	long := "needle " + strings.Repeat("x", findMaxLineBytes+20)
	findTestWriteFile(t, dir, "long.txt", "before\n"+long+"\nafter\n")

	out := findTestRun(t, findOptions{Pattern: "needle", Root: dir, Context: 1, Limit: 2, MaxFiles: 10, JSON: true})
	var got FindResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, out)
	}
	if !got.Truncated || got.EmittedLineCount != 2 {
		t.Fatalf("truncation/line count unexpected: %+v", got)
	}
	if len(got.Matches) != 1 || len(got.Matches[0].Lines) != 2 {
		t.Fatalf("lines unexpected: %+v", got.Matches)
	}
	if !got.Matches[0].Lines[1].TextTruncated || !strings.HasSuffix(got.Matches[0].Lines[1].Text, "…") {
		t.Fatalf("long line not marked truncated: %+v", got.Matches[0].Lines[1])
		if !utf8.ValidString(got.Matches[0].Lines[1].Text) {
			t.Fatalf("truncated text is not valid utf8: %q", got.Matches[0].Lines[1].Text)
		}
	}
}

func TestCmdFindSkipsDefaultsBinaryAndSymlink(t *testing.T) {
	dir := t.TempDir()
	findTestWriteFile(t, dir, "src/hit.txt", "needle\n")
	findTestWriteFile(t, dir, "node_modules/pkg/hit.txt", "needle dependency\n")
	findTestWriteFile(t, dir, "binary.bin", string([]byte{'n', 'e', 'e', 'd', 'l', 'e', 0x00}))
	findTestWriteFile(t, dir, "huge.txt", "needle "+strings.Repeat("x", findMaxFileBytes+1))
	if err := os.Symlink(filepath.Join(dir, "src"), filepath.Join(dir, "link-src")); err != nil {
		t.Logf("symlink skipped by platform: %v", err)
	}

	out := findTestRun(t, findOptions{Pattern: "needle", Root: dir, Limit: 20, MaxFiles: 20, JSON: true})
	var got FindResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, out)
	}
	if len(got.Matches) != 1 || got.Matches[0].File != "src/hit.txt" {
		t.Fatalf("default ignores/symlink/binary skip failed: %+v", got)
	}
	if got.FilesSkipped < 2 {
		t.Fatalf("filesSkipped = %d, want at least binary + ignored dir", got.FilesSkipped)
	}
}

func TestCmdFindGlobSemanticsAndZeroMatches(t *testing.T) {
	dir := t.TempDir()
	findTestWriteFile(t, dir, "a.go", "needle go\n")
	findTestWriteFile(t, dir, "a.txt", "needle txt\n")
	findTestWriteFile(t, dir, "src/b.go", "needle nested go\n")
	findTestWriteFile(t, dir, "build/c.go", "needle build\n")

	out := findTestRun(t, findOptions{Pattern: "needle", Root: dir, Limit: 10, MaxFiles: 10, Includes: []string{"*.go"}, Excludes: []string{"src/*"}, JSON: true})
	var got FindResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v\n%s", err, out)
	}
	if len(got.Matches) != 1 || got.Matches[0].File != "a.go" {
		t.Fatalf("glob include/exclude unexpected: %+v", got.Matches)
	}

	out = findTestRun(t, findOptions{Pattern: "needle", Root: dir, Limit: 10, MaxFiles: 10, Excludes: []string{"build"}, JSON: true})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal dir exclude: %v\n%s", err, out)
	}
	for _, match := range got.Matches {
		if strings.HasPrefix(match.File, "build/") {
			t.Fatalf("directory exclude did not prune build/: %+v", got.Matches)
		}
	}

	out = findTestRun(t, findOptions{Pattern: "absent", Root: dir, Limit: 10, MaxFiles: 10, JSON: true})
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json.Unmarshal no match: %v\n%s", err, out)
	}
	if !got.OK || got.Truncated || len(got.Matches) != 0 || got.MatchCount != 0 {
		t.Fatalf("zero match result unexpected: %+v", got)
	}
}

func TestRunFindHelpAndDashPattern(t *testing.T) {
	dir := t.TempDir()
	findTestWriteFile(t, dir, "dash.txt", "-needle\n")

	stdout, stderr, code := mainTestRunForCode(t, "find", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "prefer --json") {
		t.Fatalf("find help code/stderr/stdout = %d/%q/%q", code, stderr, stdout)
	}

	stdout, stderr, code = mainTestRunForCode(t, "find", "--", "-needle", dir)
	if code != 0 || stderr != "" || !strings.Contains(stdout, "-needle") {
		t.Fatalf("find dash pattern code/stderr/stdout = %d/%q/%q", code, stderr, stdout)
	}

	_, stderr, code = mainTestRunForCode(t, "find", "", dir)
	if code != 2 || !strings.Contains(stderr, "find pattern must not be empty") {
		t.Fatalf("find empty pattern code/stderr = %d/%q", code, stderr)
	}
}
