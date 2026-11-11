package main

import (
	"os"
	"path/filepath"
	"testing"
)

// formatted settles the text the way the command does, so a case describes the
// file and the shape it has to end in.
func formatted(t *testing.T, raw string) string {
	t.Helper()
	out, err := settle(raw)
	if err != nil {
		t.Fatalf("settling: %v", err)
	}
	return out
}

// TestAnArgumentListThatSpansLinesHangs settles the shape a nested expression
// takes: every parenthesis that does not open and close on one line takes its
// own line, each argument sits below the call, and the closing parenthesis goes
// back to the level of the line that opened the group.
func TestAnArgumentListThatSpansLinesHangs(t *testing.T) {
	raw := `SELECT SUM(GREATEST(a - COALESCE((
	SELECT SUM(size)
	FROM t
), 0), 0))
FROM t;
`
	want := `SELECT SUM(
	GREATEST(
		a - COALESCE(
			(
				SELECT SUM(size)
				FROM t
			),
			0
		),
		0
	)
)
FROM t;
`
	if got := formatted(t, raw); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestASingleLineListKeepsItsLine is the other half of the rule: a group that
// opens and closes on one line is left alone, however many arguments it has.
func TestASingleLineListKeepsItsLine(t *testing.T) {
	raw := `SELECT COALESCE(total_pages, 0)
FROM t
WHERE a = NOW() AND (b = 1 OR c = 2);
`
	if got, want := formatted(t, raw), raw; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestEveryConditionOfAClauseKeepsItsLevel settles the indentation of a WHERE
// whose conditions are split by groups and by an arithmetic continuation: all
// of them belong to the same clause, so they sit at the same level.
func TestEveryConditionOfAClauseKeepsItsLevel(t *testing.T) {
	raw := `SELECT 1
FROM t
WHERE a = 1
	AND (
		b = 2
		OR c = 3
	)
AND d = 4
	+ COALESCE((
		SELECT SUM(x)
		FROM u
	), 0);
`
	want := `SELECT 1
FROM t
WHERE a = 1
	AND (
		b = 2
		OR c = 3
	)
	AND d = 4
	+ COALESCE(
		(
			SELECT SUM(x)
			FROM u
		),
		0
	);
`
	if got := formatted(t, raw); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestEveryFixtureIsSettled keeps the corpus canonical: a fixture stands for
// what the rule produces, so one the formatter would change is either stale or
// a regression in the rule itself.
func TestEveryFixtureIsSettled(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "*", "*.sql"))
	if err != nil {
		t.Fatalf("listing the fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no fixtures under testdata")
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		settled, err := settle(string(raw))
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if settled != string(raw) {
			t.Errorf("%s: the fixture is not settled:\n%s", path, arrow(settled))
		}
	}
}
