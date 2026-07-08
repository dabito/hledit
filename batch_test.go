package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func batchTestRun(t *testing.T, target string, req BatchEditRequest, checkOnly bool) string {
	t.Helper()

	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	oldStdin := os.Stdin
	oldStdout := os.Stdout
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = inR
	os.Stdout = outW
	defer func() {
		os.Stdin = oldStdin
		os.Stdout = oldStdout
	}()

	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&out, outR)
		close(done)
	}()

	go func() {
		_, _ = inW.Write(payload)
		_ = inW.Close()
	}()

	if err := cmdBatch(target, checkOnly); err != nil {
		t.Fatalf("cmdBatch returned error: %v", err)
	}

	_ = outW.Close()
	_ = inR.Close()
	<-done
	_ = outR.Close()

	return strings.TrimSpace(out.String())
}

func batchTestMustUnmarshal[T any](t *testing.T, out string, target *T) {
	t.Helper()
	if err := json.Unmarshal([]byte(out), target); err != nil {
		t.Fatalf("unmarshal batch output %q: %v", out, err)
	}
}

func batchTestReadLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return []string{}
	}
	return strings.Split(text, "\n")
}

func batchTestWriteReq(t *testing.T, target string, edits ...BatchEditOp) string {
	t.Helper()
	out := batchTestRun(t, target, BatchEditRequest{Edits: edits}, false)
	if out == "" {
		t.Fatal("batch produced empty output")
	}
	return out
}

func batchTestCheckReq(t *testing.T, target string, edits ...BatchEditOp) string {
	t.Helper()
	out := batchTestRun(t, target, BatchEditRequest{Edits: edits}, true)
	if out == "" {
		t.Fatal("batch --check produced empty output")
	}
	return out
}

func TestCmdBatch(t *testing.T) {
	t.Run("replace range uses end_pos", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie", "delta")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:     "replace",
			Pos:    formatTag(2, "bravo"),
			EndPos: formatTag(3, "charlie"),
			Lines:  []string{"delta"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.EditsApplied != 1 || got.LinesAdded != 1 || got.LinesDeleted != 2 {
			t.Fatalf("batch metadata = %#v; want editsApplied 1 lines +1 -2", got)
		}
		if strings.Contains(out, "delta") {
			t.Fatalf("batch output leaked edit body: %q", out)
		}
		if want := []string{"alpha", "delta", "delta"}; !equalLines(batchTestReadLines(t, target), want) {
			t.Fatalf("target lines = %#v, want %#v", batchTestReadLines(t, target), want)
		}
	})

	t.Run("delete range uses end_pos", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie", "delta")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:     "delete",
			Pos:    formatTag(2, "bravo"),
			EndPos: formatTag(4, "delta"),
			Lines:  nil,
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LinesAdded != 0 || got.LinesDeleted != 3 {
			t.Fatalf("batch line deltas = +%d -%d; want +0 -3", got.LinesAdded, got.LinesDeleted)
		}
		if want := []string{"alpha"}; !equalLines(batchTestReadLines(t, target), want) {
			t.Fatalf("target lines = %#v, want %#v", batchTestReadLines(t, target), want)
		}
	})

	t.Run("firstChangedLine is the minimum across edits", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")

		out := batchTestWriteReq(t, target,
			BatchEditOp{OP: "replace", Pos: formatTag(3, "charlie"), Lines: []string{"CHARLIE"}},
			BatchEditOp{OP: "replace", Pos: formatTag(2, "bravo"), Lines: []string{"BRAVO"}},
		)

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if want := []string{"alpha", "BRAVO", "CHARLIE"}; !equalLines(batchTestReadLines(t, target), want) {
			t.Fatalf("target lines = %#v, want %#v", batchTestReadLines(t, target), want)
		}
	})

	t.Run("range with end_pos before pos is invalid", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:     "replace",
			Pos:    formatTag(3, "charlie"),
			EndPos: formatTag(2, "bravo"),
			Lines:  []string{"delta"},
		})

		var got BatchEditError
		batchTestMustUnmarshal(t, out, &got)
		if got.OK || got.Error != "invalid" {
			t.Fatalf("batch output = %#v; want invalid error", got)
		}
		if !strings.Contains(got.Message, "start line 3 > end line 2") {
			t.Fatalf("message = %q, want start/end detail", got.Message)
		}
		if want := []string{"alpha", "bravo", "charlie"}; !equalLines(batchTestReadLines(t, target), want) {
			t.Fatalf("target lines = %#v, want %#v", batchTestReadLines(t, target), want)
		}
	})

	t.Run("insert with empty lines is invalid", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:    "insert",
			Pos:   formatTag(1, "alpha"),
			Lines: nil,
		})

		var got BatchEditError
		batchTestMustUnmarshal(t, out, &got)
		if got.OK || got.Error != "invalid" {
			t.Fatalf("batch output = %#v; want invalid error", got)
		}
		if !strings.Contains(got.Message, "insert requires non-empty content") {
			t.Fatalf("message = %q, want empty insert detail", got.Message)
		}
		if want := []string{"alpha", "bravo"}; !equalLines(batchTestReadLines(t, target), want) {
			t.Fatalf("target lines = %#v, want %#v", batchTestReadLines(t, target), want)
		}
	})
	t.Run("stale anchor returns remaps", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")
		if err := os.WriteFile(target, []byte("alpha\nmodified\ncharlie\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:    "replace",
			Pos:   formatTag(2, "bravo"),
			Lines: []string{"NEW"},
		})

		var got BatchEditError
		batchTestMustUnmarshal(t, out, &got)
		if got.OK {
			t.Fatalf("batch succeeded unexpectedly: %#v", got)
		}
		if got.Error != "stale" {
			t.Fatalf("error = %q, want stale", got.Error)
		}
		if len(got.Remaps) == 0 {
			t.Fatalf("expected remaps, got %#v", got)
		}
	})
}

func equalLines(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestCmdBatchLastChangedLine(t *testing.T) {
	t.Run("single replace includes lastChangedLine", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:    "replace",
			Pos:   formatTag(2, "bravo"),
			Lines: []string{"BRAVO"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LastChangedLine != 2 {
			t.Fatalf("lastChangedLine = %d, want 2", got.LastChangedLine)
		}
	})

	t.Run("replace range lastChangedLine reflects end_pos", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie", "delta")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:     "replace",
			Pos:    formatTag(2, "bravo"),
			EndPos: formatTag(4, "delta"),
			Lines:  []string{"BRAVO"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LastChangedLine != 4 {
			t.Fatalf("lastChangedLine = %d, want 4", got.LastChangedLine)
		}
	})

	t.Run("insert lastChangedLine covers inserted block", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")

		out := batchTestWriteReq(t, target, BatchEditOp{
			OP:    "insert",
			Pos:   formatTag(2, "bravo"),
			Lines: []string{"one", "two"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LastChangedLine != 3 {
			t.Fatalf("lastChangedLine = %d, want 3", got.LastChangedLine)
		}
	})

	t.Run("multi-edit lastChangedLine is max across edits", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie", "delta")

		out := batchTestWriteReq(t, target,
			BatchEditOp{OP: "replace", Pos: formatTag(1, "alpha"), Lines: []string{"ALPHA"}},
			BatchEditOp{OP: "replace", Pos: formatTag(4, "delta"), Lines: []string{"DELTA"}},
		)

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("batch failed: %#v", got)
		}
		if got.FirstChangedLine != 1 {
			t.Fatalf("firstChangedLine = %d, want 1", got.FirstChangedLine)
		}
		if got.LastChangedLine != 4 {
			t.Fatalf("lastChangedLine = %d, want 4", got.LastChangedLine)
		}
	})
}

func TestCmdBatchCheck(t *testing.T) {
	t.Run("check mode succeeds and does not write", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")
		originalContent, _ := os.ReadFile(target)

		out := batchTestCheckReq(t, target, BatchEditOp{
			OP:    "replace",
			Pos:   formatTag(2, "bravo"),
			Lines: []string{"BRAVO"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK {
			t.Fatalf("check mode failed: %#v", got)
		}
		if !got.Checked {
			t.Fatalf("checked = false; want true")
		}
		if got.EditsApplied != 1 {
			t.Fatalf("editsApplied = %d, want 1", got.EditsApplied)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LastChangedLine != 2 {
			t.Fatalf("lastChangedLine = %d, want 2", got.LastChangedLine)
		}
		if got.LinesAdded != 1 || got.LinesDeleted != 1 {
			t.Fatalf("check mode line deltas = +%d -%d; want +1 -1", got.LinesAdded, got.LinesDeleted)
		}

		// File must be unchanged
		afterContent, _ := os.ReadFile(target)
		if string(afterContent) != string(originalContent) {
			t.Fatalf("check mode wrote to file: got %q, want %q", string(afterContent), string(originalContent))
		}
	})

	t.Run("check mode with range op reports lastChangedLine", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie", "delta")

		out := batchTestCheckReq(t, target, BatchEditOp{
			OP:     "replace",
			Pos:    formatTag(2, "bravo"),
			EndPos: formatTag(3, "charlie"),
			Lines:  []string{"MIDDLE"},
		})

		var got BatchEditResult
		batchTestMustUnmarshal(t, out, &got)
		if !got.OK || !got.Checked {
			t.Fatalf("check result = %#v; want ok+checked", got)
		}
		if got.FirstChangedLine != 2 {
			t.Fatalf("firstChangedLine = %d, want 2", got.FirstChangedLine)
		}
		if got.LastChangedLine != 3 {
			t.Fatalf("lastChangedLine = %d, want 3", got.LastChangedLine)
		}
		if got.LinesAdded != 1 || got.LinesDeleted != 2 {
			t.Fatalf("check range line deltas = +%d -%d; want +1 -2", got.LinesAdded, got.LinesDeleted)
		}
	})

	t.Run("check mode stale anchor does not write and reports stale", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo", "charlie")
		if err := os.WriteFile(target, []byte("alpha\nmodified\ncharlie\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalContent, _ := os.ReadFile(target)

		out := batchTestCheckReq(t, target, BatchEditOp{
			OP:    "replace",
			Pos:   formatTag(2, "bravo"),
			Lines: []string{"NEW"},
		})

		var got BatchEditError
		batchTestMustUnmarshal(t, out, &got)
		if got.OK || got.Error != "stale" {
			t.Fatalf("check mode stale: got %#v; want stale error", got)
		}
		if len(got.Remaps) == 0 {
			t.Fatalf("expected remaps, got none")
		}

		afterContent, _ := os.ReadFile(target)
		if string(afterContent) != string(originalContent) {
			t.Fatalf("check mode stale wrote to file unexpectedly")
		}
	})

	t.Run("check mode invalid op does not write", func(t *testing.T) {
		dir := t.TempDir()
		target := editTestWriteLinesFile(t, dir, "target.txt", "alpha", "bravo")
		originalContent, _ := os.ReadFile(target)

		out := batchTestCheckReq(t, target, BatchEditOp{
			OP:    "bogus",
			Pos:   formatTag(1, "alpha"),
			Lines: []string{"X"},
		})

		var got BatchEditError
		batchTestMustUnmarshal(t, out, &got)
		if got.OK || got.Error != "invalid" {
			t.Fatalf("check mode invalid: got %#v; want invalid error", got)
		}

		afterContent, _ := os.ReadFile(target)
		if string(afterContent) != string(originalContent) {
			t.Fatalf("check mode invalid wrote to file unexpectedly")
		}
	})
}
