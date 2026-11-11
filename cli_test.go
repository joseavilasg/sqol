package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectSkipsTheTreesABuildOwns(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"node_modules/pkg", ".git", "bin", "out", "queries"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{
		"node_modules/pkg/a.sql",
		".git/b.sql",
		"bin/c.sql",
		"out/d.sql",
		"queries/e.sql",
	} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collect([]string{root})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "e.sql" {
		t.Errorf("collected %v, want only queries/e.sql", files)
	}
}

func TestCollectWalksADirectoriesForSQLFiles(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.sql", filepath.Join("nested", "b.sql"), "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("SELECT 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, err := collect([]string{root})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("collected %d files, want the two .sql ones: %v", len(files), files)
	}
}

func TestCollectKeepsANamedFileWhateverItsExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.txt")
	if err := os.WriteFile(path, []byte("SELECT 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := collect([]string{path})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Errorf("collected %v, want the named file", files)
	}
}

func TestWriteFormatsAFileInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.sql")
	if err := os.WriteFile(path, []byte("select 1 from t where a and b;\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := rewrite(path, false, true, false)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if !changed {
		t.Error("rewrite reported no change for a loose statement")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "SELECT 1 FROM t WHERE a AND b;\n"; string(got) != want {
		t.Errorf("file holds %q, want %q", got, want)
	}

	again, err := rewrite(path, false, false, false)
	if err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if again {
		t.Error("rewrite reported a change on an already formatted file")
	}
}
