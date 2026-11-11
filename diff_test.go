package main

import (
	"strings"
	"testing"
)

func TestUnifiedDiffIsEmptyWhenNothingChanges(t *testing.T) {
	if got := unifiedDiff("SELECT 1\n", "SELECT 1\n", "q.sql"); got != "" {
		t.Errorf("got a diff for equal texts:\n%s", got)
	}
}

func TestUnifiedDiffShowsAChangedLine(t *testing.T) {
	got := unifiedDiff("a\nb\nc\n", "a\nB\nc\n", "q.sql")
	want := "--- q.sql\n" +
		"+++ q.sql\n" +
		"@@ -1,3 +1,3 @@\n" +
		" a\n" +
		"-b\n" +
		"+B\n" +
		" c\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestUnifiedDiffSplitsChangesThatAreFarApart(t *testing.T) {
	before := strings.Repeat("kept\n", 20) + "old\n" + strings.Repeat("kept\n", 20)
	after := strings.Repeat("kept\n", 20) + "new\n" + strings.Repeat("kept\n", 20)

	got := unifiedDiff(before, after, "q.sql")
	if n := strings.Count(got, "@@ -"); n != 1 {
		t.Errorf("a single change produced %d hunks:\n%s", n, got)
	}
	if strings.Count(got, "\n-") > 2 || strings.Count(got, "\n+") > 2 {
		t.Errorf("the hunk carries more than the changed line:\n%s", got)
	}
}

func TestUnifiedDiffCountsLinesOnBothSides(t *testing.T) {
	// The change adds a line, so the new side is one line longer than the old.
	got := unifiedDiff("a\nc\n", "a\nb\nc\n", "q.sql")
	if !strings.Contains(got, "@@ -1,2 +1,3 @@") {
		t.Errorf("wrong hunk header:\n%s", got)
	}
}
