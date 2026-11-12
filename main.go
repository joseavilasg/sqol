// Command sqol rewrites SQL files with tabs, one clause per line and one
// indentation level per nesting step.
//
// It formats the paths it is given: a file as it is, and every .sql file under
// a directory. Without paths it walks the working directory.
//
// Usage: sqol [-l] [-w] [-d] [-stdin] [path ...]
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/joseavilasg/sqol/formatter"
)

func main() {
	list := flag.Bool("l", false, "list the files whose layout would change")
	write := flag.Bool("w", false, "write the result to each file instead of standard output")
	showDiff := flag.Bool("d", false, "print the changes as a unified diff")
	fromStdin := flag.Bool("stdin", false, "read one document from standard input and print the result")
	flag.Parse()

	if *fromStdin {
		if err := formatStream(); err != nil {
			fail(err)
		}
		return
	}
	if *write && *showDiff {
		fail(fmt.Errorf("-w and -d cannot be combined: one writes, the other reports"))
	}

	files, err := collect(flag.Args())
	if err != nil {
		fail(err)
	}

	differed := 0
	for _, path := range files {
		changed, err := rewrite(path, *list, *write, *showDiff)
		if err != nil {
			fail(err)
		}
		if changed {
			differed++
		}
	}
	if differed > 0 && (*list || *showDiff) {
		os.Exit(1)
	}
}

// fail reports an error the way a command line tool does and leaves.
func fail(err error) {
	fmt.Fprintln(os.Stderr, "sqol:", err)
	os.Exit(2)
}

// collect lists the files to format: a named file as it is, and every .sql
// file under a named directory. Without arguments it walks the working
// directory.
func collect(args []string) ([]string, error) {
	if len(args) == 0 {
		args = []string{"."}
	}
	var files []string
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			files = append(files, arg)
			continue
		}
		found, err := walkSQL(arg)
		if err != nil {
			return nil, err
		}
		files = append(files, found...)
	}
	return files, nil
}

// walkSQL returns every .sql file under root, skipping the trees a build and a
// version control system own.
func walkSQL(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		return visitSQL(path, root, entry, err, &files)
	})
	return files, err
}

// visitSQL adds one entry to the list when it is a .sql file.
func visitSQL(path, root string, entry fs.DirEntry, err error, files *[]string) error {
	if err != nil {
		return err
	}
	if entry.IsDir() {
		// A directory named on the command line is always walked, but the ones
		// inside it are not: version control and build output hold no SQL worth
		// formatting, and walking them costs more than it finds.
		if path != root && skippedDir(entry.Name()) {
			return filepath.SkipDir
		}
		return nil
	}
	if strings.HasSuffix(path, ".sql") {
		*files = append(*files, path)
	}
	return nil
}

// skippedDir reports a directory the walk does not enter: the hidden ones,
// which carry tool state, and the three trees a build writes.
func skippedDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules" || name == "bin" || name == "out"
}

// rewrite formats one file and applies whichever result the flags asked for.
// It reports whether the text changed.
func rewrite(path string, list, write, showDiff bool) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := formatter.Settle(string(raw))
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if formatted == string(raw) {
		return false, nil
	}
	switch {
	case showDiff:
		fmt.Print(unifiedDiff(string(raw), formatted, path))
	case list:
		fmt.Println(path)
	}
	if write {
		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			return true, err
		}
	}
	if !list && !write && !showDiff {
		fmt.Print(formatted)
	}
	return true, nil
}

// formatStream formats one document read from standard input.
func formatStream() error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	formatted, err := formatter.Settle(string(raw))
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(formatted)
	return err
}
