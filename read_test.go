package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readTestCaptureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	outCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		outCh <- string(b)
	}()

	fn()
	_ = w.Close()
	out := <-outCh
	_ = r.Close()
	return out
}

func readTestLines(t *testing.T, output string) []string {
	t.Helper()
	output = strings.TrimSuffix(output, "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func readTestWriteFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
	return path
}

func TestCmdReadAnnotatesSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "small.txt", "alpha\nbeta\n")

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	want := []string{
		formatTag(1, "alpha") + ":alpha",
		formatTag(2, "beta") + ":beta",
	}
	if len(got) != len(want) {
		t.Fatalf("cmdRead output line count = %d; want %d (%q)", len(got), len(want), output)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("cmdRead line %d = %q; want %q", i+1, got[i], want[i])
		}
	}
}

func TestCmdReadDropsTrailingPhantomEmptyLine(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "trailing-newline.txt", "solo\n")

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	if len(got) != 1 {
		t.Fatalf("cmdRead output line count = %d; want 1 (%q)", len(got), output)
	}
	want := formatTag(1, "solo") + ":solo"
	if got[0] != want {
		t.Fatalf("cmdRead line = %q; want %q", got[0], want)
	}
}

func TestCmdReadMissingFileEmitsIOErrorJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.txt")

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	var got EditError
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v (output=%q)", err, output)
	}
	if got.OK {
		t.Fatalf("cmdRead missing file ok = true; want false")
	}
	if got.Error != "io" {
		t.Fatalf("cmdRead missing file error = %q; want %q", got.Error, "io")
	}
}

func TestCmdReadBinaryDetectionEmitsJSONError(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "binary.bin", string([]byte{'a', 0x00, 'b', '\n'}))

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	var got EditError
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v (output=%q)", err, output)
	}
	if got.OK {
		t.Fatalf("cmdRead binary ok = true; want false")
	}
	if got.Error != "binary" {
		t.Fatalf("cmdRead binary error = %q; want %q", got.Error, "binary")
	}
}

func TestCmdReadTruncatesAfterTwoThousandLines(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 2001; i++ {
		b.WriteString("x\n")
	}
	path := readTestWriteFile(t, dir, "many-lines.txt", b.String())

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	if len(got) != 2001 {
		t.Fatalf("cmdRead output line count = %d; want 2001", len(got))
	}
	if got[0] != formatTag(1, "x")+":x" {
		t.Fatalf("cmdRead first line = %q; want tagged line 1", got[0])
	}
	if got[1999] != formatTag(2000, "x")+":x" {
		t.Fatalf("cmdRead 2000th line = %q; want tagged line 2000", got[1999])
	}
	if got[2000] != "-- truncated: use read-range --offset 2001 --" {
		t.Fatalf("cmdRead truncation notice = %q; want offset 2001", got[2000])
	}
}

func TestCmdReadRangeUsesAbsoluteLineNumbersAndExactRange(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "range.txt", "one\ntwo\nthree\nfour\nfive\n")

	output := readTestCaptureStdout(t, func() {
		if err := cmdReadRange(path, 2, 2, ""); err != nil {
			t.Fatalf("cmdReadRange returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	if len(got) != 3 {
		t.Fatalf("cmdReadRange output line count = %d; want 3 (%q)", len(got), output)
	}
	if got[0] != formatTag(2, "two")+":two" {
		t.Fatalf("cmdReadRange first line = %q; want line 2", got[0])
	}
	if got[1] != formatTag(3, "three")+":three" {
		t.Fatalf("cmdReadRange second line = %q; want line 3", got[1])
	}
	if got[2] != "-- truncated: use read-range --offset 4 --" {
		t.Fatalf("cmdReadRange truncation notice = %q; want offset 4", got[2])
	}
}

func TestCmdReadRangeOffsetLessThanOneIsTreatedAsOne(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "offset0.txt", "first\nsecond\n")

	output := readTestCaptureStdout(t, func() {
		if err := cmdReadRange(path, 0, 1, ""); err != nil {
			t.Fatalf("cmdReadRange returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	if len(got) != 2 {
		t.Fatalf("cmdReadRange output line count = %d; want 2 (%q)", len(got), output)
	}
	want := formatTag(1, "first") + ":first"
	if got[0] != want {
		t.Fatalf("cmdReadRange first line = %q; want %q", got[0], want)
	}
	if got[1] != "-- truncated: use read-range --offset 2 --" {
		t.Fatalf("cmdReadRange truncation notice = %q; want offset 2", got[1])
	}
}

func TestCmdReadRangeOffsetBeyondFileLengthEmitsRangeError(t *testing.T) {
	dir := t.TempDir()
	path := readTestWriteFile(t, dir, "range-error.txt", "one\ntwo\n")

	output := readTestCaptureStdout(t, func() {
		if err := cmdReadRange(path, 3, 1, ""); err != nil {
			t.Fatalf("cmdReadRange returned error: %v", err)
		}
	})

	var got EditError
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v (output=%q)", err, output)
	}
	if got.OK {
		t.Fatalf("cmdReadRange range ok = true; want false")
	}
	if got.Error != "range" {
		t.Fatalf("cmdReadRange range error = %q; want %q", got.Error, "range")
	}
}

func TestCmdReadTruncatesAt50KBByteLimit(t *testing.T) {
	dir := t.TempDir()
	// Create lines that are each ~100 bytes. 50 KB = 51200 bytes.
	// With ~100 bytes per line (including tag prefix), we should hit
	// the byte limit well before 2000 lines.
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		b.WriteString("this is a deliberately long line to test byte-limit truncation behavior\n")
	}
	path := readTestWriteFile(t, dir, "big-lines.txt", b.String())

	output := readTestCaptureStdout(t, func() {
		if err := cmdRead(path); err != nil {
			t.Fatalf("cmdRead returned error: %v", err)
		}
	})

	got := readTestLines(t, output)
	// Should have been truncated by byte limit, not line count
	if len(got) >= 2000 {
		t.Fatalf("cmdRead output line count = %d; expected < 2000 (byte-limit truncation)", len(got))
	}
	// Last line should be the truncation notice
	lastLine := got[len(got)-1]
	if !strings.HasPrefix(lastLine, "-- truncated: use read-range --offset") {
		t.Fatalf("cmdRead last line = %q; want truncation notice", lastLine)
	}
	// Verify the output byte size is near the 50 KB limit
	if len(output) > 50*1024+1024 {
		t.Fatalf("cmdRead output size = %d; expected near 50 KB", len(output))
	}
}
