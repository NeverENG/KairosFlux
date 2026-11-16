package main

import "testing"

func TestRunRequiresRegistryPath(t *testing.T) {
	if got := run(nil); got != 2 {
		t.Fatalf("missing path: got exit %d, want 2", got)
	}
}

func TestRunReportsMissingFile(t *testing.T) {
	if got := run([]string{"/nonexistent/registry.jsonl"}); got != 1 {
		t.Fatalf("missing file: got exit %d, want 1", got)
	}
}
