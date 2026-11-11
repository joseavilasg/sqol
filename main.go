// Command sqol rewrites SQL files with tabs, one clause per line and one
// indentation level per nesting step.
//
// The rule is the one the files follow, written down. A clause keyword starts at
// its block's level and what the clause holds sits one level below. FROM and
// every JOIN share a level; an ON and its AND sit one level below the JOIN that
// opened them, and the AND of a WHERE or HAVING sits one below the clause. A
// statement that occupies several lines indents its continuations one level past
// the line that started it. A procedural body keeps its own blocks: BEGIN, IF,
// LOOP and CASE indent what they hold, ELSIF, ELSE and END return to the level
// of the line that opened them. A FOR or WHILE header that ends with LOOP
// opens one too.
//
// Three shapes have a rule of their own. The columns of a CREATE TABLE line up
// in three fields: the name, the type, and then NOT NULL or the default. The ON
// of a CREATE INDEX sits one level under the CREATE, and a WHERE that filters
// the index returns to the index's own level. Each branch of a UNION is
// separated from the keyword by a blank line.
//
// The transform is whitespace only. String literals, quoted identifiers and
// comments keep their text; a line is never reflowed, only reindented, except a
// SELECT whose list continues on the next line, which moves the list under the
// keyword.
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
	"regexp"
	"strings"
)

var (
	keywordRe = regexp.MustCompile(`^(SELECT|FROM|WHERE|GROUP\s+BY|ORDER\s+BY|HAVING|LIMIT|OFFSET|RETURNING|VALUES|SET|UNION|EXCEPT|INTERSECT|INSERT|UPDATE|DELETE|WITH|JOIN|LEFT|RIGHT|INNER|FULL|CROSS|ON|USING|WINDOW)\b`)
	starterRe = regexp.MustCompile(`^(SELECT|INSERT|UPDATE|DELETE|WITH)\b`)
	selectRe  = regexp.MustCompile(`^SELECT\b`)
	unionRe   = regexp.MustCompile(`\bUNION(\s+ALL)?\b`)
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
		err = filepath.WalkDir(arg, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".sql") {
				return err
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

// rewrite formats one file and applies whichever result the flags asked for.
// It reports whether the text changed.
func rewrite(path string, list, write, showDiff bool) (bool, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := settle(string(raw))
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
	formatted, err := settle(string(raw))
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(formatted)
	return err
}

// settle applies the rewrite until the text stops changing. One pass is enough
// when the file already follows the shape; a file written by an older version of
// this tool can need a second, because a continuation sits relative to the line
// that started its statement and that line moves in the first pass.
func settle(raw string) (string, error) {
	current := raw
	for range 20 {
		next := format(current)
		if next == current {
			return next, nil
		}
		current = next
	}
	return current, fmt.Errorf("the layout does not settle after five passes")
}

// source is one line with two extra views: trimmed, and masked, which has
// literals and comments blanked so detection never reads inside them.
type source struct {
	text    string
	trim    string
	masked  string
	blank   bool
	comment bool
}

func readLines(raw string) []source {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]source, len(lines))
	for i, text := range lines {
		m := mask(text)
		trim := strings.TrimSpace(text)
		out[i] = source{
			text: text, trim: trim, masked: m,
			blank:   trim == "",
			comment: strings.HasPrefix(trim, "--"),
		}
	}
	return out
}

// mask blanks quoted text, quoted identifiers and comments. The result keeps the
// length of its input so positions line up.
func mask(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSingle, inDouble, inLine, inBlock := false, false, false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inLine:
			if c == '\n' {
				b.WriteByte('\n')
				inLine = false
			} else {
				b.WriteByte(' ')
			}
		case inBlock:
			b.WriteByte(' ')
			if c == '*' && i+1 < len(s) && s[i+1] == '/' {
				b.WriteByte(' ')
				i++
				inBlock = false
			}
		case inSingle:
			b.WriteByte(' ')
			if c == '\'' {
				if i+1 < len(s) && s[i+1] == '\'' {
					b.WriteByte(' ')
					i++
				} else {
					inSingle = false
				}
			}
		case inDouble:
			b.WriteByte(' ')
			if c == '"' {
				inDouble = false
			}
		case c == '-' && i+1 < len(s) && s[i+1] == '-':
			b.WriteString("  ")
			i++
			inLine = true
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			b.WriteString("  ")
			i++
			inBlock = true
		case c == '\'':
			b.WriteByte(' ')
			inSingle = true
		case c == '"':
			b.WriteByte(' ')
			inDouble = true
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func tabs(n int) string { return strings.Repeat("\t", max(n, 0)) }

// openParen is one parenthesis the formatter has not closed yet: the level of
// the line that opened it, and whether a clause condition was running when it
// did. Closing a group restores that condition, so the AND of a WHERE keeps its
// level after a group inside the condition ends.
type openParen struct {
	indent    int
	inClause  bool
	andIndent int
}

// block is one procedural scope: what opened it and the level it sits at.
type block struct {
	kind   string
	indent int
}

type formatter struct {
	parens     []openParen // the line that opened each parenthesis
	blocks     []block
	inBody     bool // inside a DO $$ ... $$ body
	stmtBase   int  // level of the first line of the current statement
	prevInd    int  // level of the previous code line
	prevTrim   string
	prevClose  bool // the previous code line closed a parenthesis
	prevClause bool // the previous code line was a clause keyword
	afterUnion bool // the next code line needs a blank line above it
	afterJoin  bool // the last clause was a JOIN, so an ON belongs to it
	opIndent   int  // the level the expression's operator lines sit at
	opSet      bool
	fresh      bool // the next line starts a statement
	inClause   bool
	andIndent  int // where AND and OR go, when a clause set it
}

// base is the level a clause keyword sits at: the block it belongs to, or one
// level inside the parenthesis that holds it, whichever is deeper.
func (f *formatter) base() int {
	level := 0
	if len(f.blocks) > 0 {
		level = f.blocks[len(f.blocks)-1].indent + 1
	}
	if len(f.parens) > 0 {
		if deeper := f.parens[len(f.parens)-1].indent + 1; deeper > level {
			level = deeper
		}
	}
	return level
}

func (f *formatter) push(kind string, indent int) {
	f.blocks = append(f.blocks, block{kind: kind, indent: indent})
}

// pop closes the innermost block and returns the level it sat at.
func (f *formatter) pop() int {
	if len(f.blocks) == 0 {
		return 0
	}
	top := f.blocks[len(f.blocks)-1]
	f.blocks = f.blocks[:len(f.blocks)-1]
	return top.indent
}

func (f *formatter) topKind() string {
	if len(f.blocks) == 0 {
		return ""
	}
	return f.blocks[len(f.blocks)-1].kind
}

// continuation is the level of a line that neither starts a clause nor opens a
// block. A statement that wraps indents its continuations one level past the
// line that started it; a list keeps its items aligned, and a keyword that
// introduces its own list, like SET, indents one step past itself.
func (f *formatter) continuation(trim string) int {
	level := f.base()
	if f.fresh {
		f.stmtBase, f.fresh, f.opSet = level, false, false
		return level
	}
	// A line that continues an expression with an operator stays beside the
	// operator it repeats, not one level deeper.
	if startsWithOperator(trim) {
		if f.opSet {
			return f.opIndent
		}
		f.opIndent, f.opSet = max(level, f.stmtBase+1), true
		return f.opIndent
	}
	if f.prevClause {
		// What a clause keyword holds sits one level below it.
		return max(level, f.prevInd+1)
	}
	if f.prevClose {
		// A continuation that follows a closed group, like the next name in a
		// WITH list, sits where the group started rather than one level in.
		f.prevClose = false
		return level
	}
	switch {
	case strings.HasSuffix(strings.ToUpper(f.prevTrim), " SET"):
		return max(level, f.prevInd+1)
	case strings.HasSuffix(f.prevTrim, ","):
		return max(level, f.prevInd)
	}
	return max(level, f.stmtBase+1)
}

// clause is the level of a clause keyword, and records where its AND and OR go.
func (f *formatter) clause(trim, upper string) int {
	level := f.base()
	switch {
	case strings.HasPrefix(upper, "ON CONFLICT"):
		// ON CONFLICT belongs to the INSERT, not to a join: it sits where the
		// statement started.
		level = f.stmtBase
		f.inClause = false
	case strings.HasPrefix(upper, "ON ") || upper == "ON":
		if !f.afterJoin {
			// The ON of CREATE INDEX names the table, not a join condition, and
			// it continues the statement one level in.
			f.inClause = false
			level = max(level, f.stmtBase+1)
			break
		}
		// ON and its AND sit one level below the JOIN they belong to, and the
		// JOIN itself one below the FROM.
		level += 2
		f.andIndent, f.inClause = level, true
	case isJoin(upper):
		// A join sits one level below the FROM it belongs to.
		level++
		f.afterJoin = true
		f.inClause = false
	case strings.HasPrefix(upper, "WHERE"), strings.HasPrefix(upper, "HAVING"):
		// The conditions of a WHERE sit one level below the keyword.
		f.andIndent, f.inClause = level+1, true
	default:
		f.inClause = false
	}
	if !isJoin(upper) && !strings.HasPrefix(upper, "ON") {
		f.afterJoin = false
	}
	// A nested SELECT starts a statement of its own, so its list indents from
	// it even when it opens inside another statement's parenthesis.
	if f.fresh || starterRe.MatchString(upper) {
		f.stmtBase = level
		f.fresh = false
	}
	return level
}

func format(raw string) string {
	// The columns of a table are lined up first: the widths they share depend on
	// every line of the body, so they cannot be decided one line at a time.
	aligned := alignColumns(readLines(uppercaseKeywords(raw)))
	text := make([]string, len(aligned))
	for i := range aligned {
		text[i] = aligned[i].text
	}
	lines := readLines(strings.Join(text, "\n"))
	f := &formatter{fresh: true}
	var out []string
	lastCode := ""

	for _, ln := range lines {
		if ln.blank {
			out = append(out, "")
			f.fresh = true
			f.inClause = false
			f.afterUnion = false
			lastCode = ""
			continue
		}
		if ln.comment {
			// A comment keeps the level of the block it sits in. Flattening it to
			// the left margin detaches it from the lines it explains. A goose
			// annotation is refused with any leading whitespace, so it always
			// starts at the margin.
			level := 0
			switch {
			case strings.HasPrefix(ln.trim, "-- +goose"):
			case len(f.blocks) > 0 || len(f.parens) > 0:
				level = f.base()
			case strings.HasSuffix(lastCode, ","):
				level = f.prevInd
			}
			out = append(out, tabs(level)+ln.trim)
			continue
		}

		trim := ln.trim
		upper := strings.ToUpper(trim)

		// A UNION separates two result sets, so it gets a blank line on each
		// side; without it the seam reads as another clause of the same query.
		if f.afterUnion {
			out = append(out, "")
			f.afterUnion = false
		}
		isUnion := upper == "UNION" || strings.HasPrefix(upper, "UNION ALL") || strings.HasPrefix(upper, "UNION ")
		if isUnion && len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}

		// A line that closes groups opened on earlier lines takes one line per
		// group, and an argument that follows the run starts below the call it
		// belongs to. A lone closing with nothing after it is left to the
		// accounting, which knows where it belongs.
		if lines, rest, restLevel, lastLevel, closes, ok := f.closeLines(trim); ok {
			out = append(out, lines...)
			f.closeParens(closes)
			if rest == "" {
				f.prevInd, f.prevTrim, f.prevClause = lastLevel, ")", false
				f.prevClose = true
				lastCode = ")"
			} else {
				out = append(out, tabs(restLevel)+rest)
				f.prevInd, f.prevTrim, f.prevClause = restLevel, rest, false
				f.prevClose = false
				lastCode = rest
				f.update(mask(rest), 0, restLevel)
			}
			continue
		}

		cur := 0
		if f.inBody {
			cur = f.body(trim, upper)
		} else {
			cur = f.statement(trim, upper)
		}

		// A group that continues on the following lines opens with a parenthesis
		// that ends its line, so the block reads as one.
		if head, rest, ok := openSplit(trim); ok {
			out = append(out, tabs(cur)+head)
			out = append(out, tabs(cur+1)+rest)
			f.open(cur)
			f.update(mask(rest), 0, cur+1)
			f.prevInd, f.prevTrim, f.prevClause = cur+1, rest, false
			lastCode = rest
			continue
		}

		// A closing parenthesis that shares its line with the statement inside it
		// takes its own line, so the group reads as a block. Only a parenthesis
		// opened on an earlier line counts; one that opens and closes here is
		// part of the expression.
		if prefix, suffix, opener, ok := f.closeSplitAt(trim, len(f.parens)); ok {
			out = append(out, tabs(cur)+prefix)
			out = append(out, tabs(opener)+suffix)
			f.update(mask(prefix), 0, cur)
			// The suffix opens with that parenthesis, so it is popped here instead
			// of being left to the stack's own accounting.
			if len(f.parens) > 0 {
				f.parens = f.parens[:len(f.parens)-1]
			}
			f.update(mask(suffix), leadingCloses(suffix), opener)
			f.prevInd, f.prevTrim, f.prevClause = opener, suffix, false
			lastCode = suffix
			continue
		}

		// A UNION that shares its line with both queries is split, so the blank
		// lines that set it apart have somewhere to go. When the union sits
		// inside parentheses that open on the same line, the parentheses take
		// their own lines too; otherwise the closing one would trail the last
		// select and read as part of it.
		if pre, parts, post, ok := splitUnionLine(trim); ok {
			emit := func(level int, text string) {
				out = append(out, tabs(level)+text)
			}
			union := func(level int, text string) {
				keyword := isUnionPart(text)
				if f.afterUnion {
					out = append(out, "")
					f.afterUnion = false
				}
				if keyword && len(out) > 0 && out[len(out)-1] != "" {
					out = append(out, "")
				}
				emit(level, text)
				f.afterUnion = keyword
			}
			if pre != "" {
				emit(cur, pre)
				for _, r := range mask(pre) {
					if r == '(' {
						f.open(cur)
					}
				}
				for _, part := range parts {
					union(cur+1, part)
					for _, r := range mask(part) {
						switch r {
						case '(':
							f.open(cur + 1)
						case ')':
							if len(f.parens) > 0 {
								f.parens = f.parens[:len(f.parens)-1]
							}
						}
					}
				}
				if post != "" {
					if f.afterUnion {
						out = append(out, "")
						f.afterUnion = false
					}
					emit(cur, post)
					for _, r := range mask(post) {
						if r == ')' {
							if len(f.parens) > 0 {
								f.parens = f.parens[:len(f.parens)-1]
							}
						}
					}
				}
			} else {
				for _, part := range parts {
					union(cur, part)
				}
				f.update(ln.masked, leadingCloses(trim), cur)
			}
			f.prevInd, f.prevTrim, f.prevClause = cur, trim, false
			lastCode = trim
			continue
		}

		if selectRe.MatchString(trim) && strings.HasSuffix(trim, ",") {
			head, rest := splitSelect(trim)
			out = append(out, tabs(cur)+head)
			if rest != "" {
				out = append(out, tabs(cur+1)+rest)
			}
			f.prevInd, f.prevTrim, f.prevClause = cur+1, rest, false
			headMask, restMask := splitSelect(ln.masked)
			f.update(headMask, 0, cur)
			f.update(restMask, 0, cur+1)
			continue
		}

		out = append(out, tabs(cur)+trim)
		lastCode = trim
		f.afterUnion = isUnion
		f.prevInd, f.prevTrim = cur, trim
		f.prevClause = keywordRe.MatchString(trim)
		if leadingCloses(trim) == 0 {
			f.prevClose = false
		}
		f.update(ln.masked, leadingCloses(trim), cur)
		// A semicolon ends the statement, so the next line starts a new one. The
		// rule matters inside a DECLARE block, where every variable is its own
		// statement and none of them is a keyword.
		if strings.HasSuffix(trim, ";") {
			f.fresh = true
		}
		if !f.inBody && strings.HasSuffix(ln.masked, "$$") {
			f.inBody = true
			f.fresh = true
		}
	}

	for len(out) > 1 && out[len(out)-1] == "" && out[len(out)-2] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// statement computes the level of a line outside a procedural body.
func (f *formatter) statement(trim, upper string) int {
	if closes := leadingCloses(trim); closes > 0 {
		f.prevClose = true
		return f.closeParens(closes)
	}
	if keywordRe.MatchString(trim) {
		return f.clause(trim, upper)
	}
	if isAndOr(upper) && f.inClause {
		// A condition group can open a parenthesis; inside it, its AND and OR
		// belong to the group's level, not to the clause's.
		return max(f.andIndent, f.base())
	}
	return f.continuation(trim)
}

// body computes the level of a line inside a DO block.
func (f *formatter) body(trim, upper string) int {
	if closes := leadingCloses(trim); closes > 0 {
		// `) THEN` ends the condition of an IF, so the body starts after it.
		shut := strings.ToUpper(strings.TrimSpace(trim[closes:]))
		if shut == "THEN" || shut == "LOOP" || shut == "DO" {
			f.fresh = true
		}
		return f.closeParens(closes)
	}

	switch {
	case strings.HasPrefix(upper, "END $$"):
		// Closes the body: the level returns to the DO statement's.
		for len(f.blocks) > 0 {
			f.pop()
		}
		f.inBody = false
		f.inClause = false
		f.fresh = true
		return 0

	case strings.HasPrefix(upper, "END"):
		f.inClause = false
		f.fresh = true
		return f.pop()

	case strings.HasPrefix(upper, "ELSIF"), upper == "ELSE":
		f.inClause = false
		f.fresh = true
		if len(f.blocks) > 0 {
			return f.blocks[len(f.blocks)-1].indent
		}
		return 0

	case strings.HasPrefix(upper, "DECLARE"):
		f.inClause = false
		f.fresh = true
		cur := f.base()
		f.push("declare", cur)
		return cur

	case strings.HasPrefix(upper, "BEGIN"), upper == "BEGIN":
		f.inClause = false
		f.fresh = true
		if f.topKind() == "declare" {
			f.pop()
		}
		cur := f.base()
		f.push("begin", cur)
		return cur

	case strings.HasPrefix(upper, "IF "), upper == "IF":
		f.inClause = false
		f.fresh = true
		cur := f.base()
		f.push("if", cur)
		return cur

	case opensLoop(upper),
		strings.HasPrefix(upper, "CASE"), upper == "CASE":
		f.inClause = false
		f.fresh = true
		cur := f.base()
		f.push("loop", cur)
		return cur
	}

	// A statement inside the body follows the same clause rules as outside.
	if keywordRe.MatchString(trim) {
		return f.clause(trim, upper)
	}
	if isAndOr(upper) && f.inClause {
		return max(f.andIndent, f.base())
	}
	return f.continuation(trim)
}

// opensLoop reports a line that opens a loop: LOOP on its own, or the FOR or
// WHILE header that ends with the keyword and puts its body on the following
// lines. END LOOP closes one, so it is not an opener.
func opensLoop(upper string) bool {
	line := strings.TrimSuffix(strings.TrimSpace(upper), ";")
	if strings.HasPrefix(line, "END") {
		return false
	}
	return line == "LOOP" || strings.HasSuffix(line, " LOOP")
}

func isJoin(upper string) bool {
	for _, prefix := range []string{"JOIN", "LEFT", "RIGHT", "INNER", "FULL", "CROSS"} {
		if upper == prefix || strings.HasPrefix(upper, prefix+" ") {
			return true
		}
	}
	return false
}

func leadingCloses(trim string) int {
	n := 0
	for n < len(trim) && trim[n] == ')' {
		n++
	}
	return n
}

// closeParens closes the innermost groups and returns the level of the deepest
// one. A group that was opened while a clause condition was running puts that
// condition back: the WHERE it belongs to is still open.
func (f *formatter) closeParens(count int) int {
	cur := 0
	if len(f.parens) == 0 {
		return cur
	}
	cur = f.parens[len(f.parens)-1].indent
	popped := make([]openParen, 0, count)
	for k := 0; k < count && len(f.parens) > 0; k++ {
		popped = append(popped, f.parens[len(f.parens)-1])
		f.parens = f.parens[:len(f.parens)-1]
	}
	if len(popped) > 0 {
		f.inClause = popped[len(popped)-1].inClause
		f.andIndent = popped[len(popped)-1].andIndent
	}
	return cur
}

func isAndOr(upper string) bool {
	return strings.HasPrefix(upper, "AND ") || upper == "AND" ||
		strings.HasPrefix(upper, "OR ") || upper == "OR"
}

func startsWithOperator(trim string) bool {
	if trim == "" {
		return false
	}
	switch trim[0] {
	case '+', '-', '*', '/':
		return true
	}
	return false
}

// splitSelect separates the keyword, and its distinct qualifier, from the
// columns it introduces. A SELECT whose list continues on the next line is the
// only one that moves: it ends with a comma, and the comma makes it a list
// rather than a complete statement.
func splitSelect(trim string) (string, string) {
	rest := strings.TrimSpace(strings.TrimPrefix(trim, "SELECT"))
	head := "SELECT"
	upper := strings.ToUpper(rest)
	switch {
	case strings.HasPrefix(upper, "DISTINCT ON ("):
		if end := strings.Index(rest, ")"); end >= 0 {
			head = "SELECT " + rest[:end+1]
			rest = strings.TrimSpace(rest[end+1:])
		}
	case strings.HasPrefix(upper, "DISTINCT "):
		head = "SELECT DISTINCT"
		rest = strings.TrimSpace(rest[len("DISTINCT"):])
	}
	return head, rest
}

// open records a parenthesis the emitter opened on a line at level.
func (f *formatter) open(level int) {
	f.parens = append(f.parens, openParen{indent: level, inClause: f.inClause, andIndent: f.andIndent})
}

// update adjusts the parenthesis stack. The leading closes already moved the
// line, so they are skipped here.
func (f *formatter) update(masked string, leadClose, openerLevel int) {
	m := strings.TrimLeft(masked, " \t")
	if leadClose > 0 && len(m) >= leadClose {
		m = m[leadClose:]
	}
	for _, c := range m {
		switch c {
		case '(':
			f.open(openerLevel)
		case ')':
			if len(f.parens) > 0 {
				f.parens = f.parens[:len(f.parens)-1]
			}
		}
	}
}

// splitUnionLine breaks a line that holds a UNION into the pieces a reader
// expects on separate lines. When the union sits inside a parenthesized
// subquery that opens on the same line, the text before the parenthesis, the
// queries, and the text from the closing parenthesis on are returned apart, so
// each takes its own line.
func splitUnionLine(trim string) (pre string, parts []string, post string, ok bool) {
	masked := mask(trim)
	matches := unionRe.FindAllStringIndex(masked, -1)
	if len(matches) == 0 {
		return "", nil, "", false
	}

	body := trim
	if open, close, found := enclosingParen(masked, matches[0][0], matches[len(matches)-1][1]); found {
		pre, body, post = trim[:open+1], trim[open+1:close], trim[close:]
	}

	bodyMask := mask(body)
	spans := unionRe.FindAllStringIndex(bodyMask, -1)
	start := 0
	for _, span := range spans {
		if before := strings.TrimSpace(body[start:span[0]]); before != "" {
			parts = append(parts, before)
		}
		parts = append(parts, strings.Join(strings.Fields(body[span[0]:span[1]]), " "))
		start = span[1]
	}
	if after := strings.TrimSpace(body[start:]); after != "" {
		parts = append(parts, after)
	}
	if len(parts) < 2 {
		return "", nil, "", false
	}
	return pre, parts, strings.TrimSpace(post), true
}

// enclosingParen finds the innermost parenthesis that contains the span from
// first to last on the same line.
func enclosingParen(masked string, first, last int) (open, close int, ok bool) {
	var stack []int
	best := -1
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) == 0 {
				continue
			}
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if o < first && i > last && (best == -1 || o > best) {
				best, close = o, i
			}
		}
	}
	if best == -1 {
		return 0, 0, false
	}
	return best, close, true
}

// isUnionPart reports whether a piece is the keyword itself.
func isUnionPart(part string) bool {
	upper := strings.ToUpper(part)
	return upper == "UNION" || strings.HasPrefix(upper, "UNION ALL") || strings.HasPrefix(upper, "UNION ")
}

// closeSplit finds a closing parenthesis on the line that closes a group opened
// on an earlier line, and that has text before it. It returns the text before it,
// from the parenthesis on, and the level of the line that opened the group.
func (f *formatter) closeSplitAt(trim string, depth int) (prefix, suffix string, opener int, ok bool) {
	masked := mask(trim)
	level := depth
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '(':
			level++
		case ')':
			level--
			if level < depth {
				before := strings.TrimSpace(trim[:i])
				if before == "" {
					return "", "", 0, false
				}
				open := f.parens[depth-1].indent
				return before, strings.TrimSpace(trim[i:]), open, true
			}
		}
	}
	return "", "", 0, false
}

// openSplit finds a parenthesis that opens a group which continues past this
// line, and that has text after it. It returns the line up to and including the
// parenthesis, and the text that follows it.
func openSplit(trim string) (head, rest string, ok bool) {
	masked := mask(trim)
	var stack []int
	closed := make(map[int]bool)
	for i := 0; i < len(masked); i++ {
		switch masked[i] {
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) > 0 {
				closed[stack[len(stack)-1]] = true
				stack = stack[:len(stack)-1]
			}
		}
	}
	for _, open := range stack {
		after := strings.TrimSpace(trim[open+1:])
		// A suffix of nothing at all is the group opening; there is no
		// statement to move down.
		if after == "" || closed[open] {
			continue
		}
		return strings.TrimSpace(trim[:open+1]), after, true
	}
	return "", "", false
}

// closeLines renders a line that begins by closing groups opened on earlier
// lines. A run of closings becomes one line per group, at the level that opened
// it, and an argument after the run starts below the call it belongs to. It
// returns nothing when the line has something that belongs on the same line — a
// keyword, a cast, a semicolon — or when a single closing with nothing after it
// is left to the accounting.
func (f *formatter) closeLines(trim string) (lines []string, rest string, restLevel, lastLevel, closes int, ok bool) {
	masked := mask(trim)
	for closes < len(masked) && masked[closes] == ')' {
		closes++
	}
	if closes == 0 || closes > len(f.parens) {
		return nil, "", 0, 0, 0, false
	}
	after := strings.TrimSpace(trim[closes:])
	comma := strings.HasPrefix(after, ",")
	rest = strings.TrimSpace(strings.TrimPrefix(after, ","))
	switch {
	case rest == "":
		if closes == 1 {
			return nil, "", 0, 0, 0, false
		}
	case !comma:
		return nil, "", 0, 0, 0, false
	}
	for j := 0; j < closes; j++ {
		lastLevel = f.parens[len(f.parens)-1-j].indent
		text := ")"
		if j == closes-1 && rest != "" {
			text = "),"
		}
		lines = append(lines, tabs(lastLevel)+text)
	}
	// What follows the run is another argument of the group that still
	// encloses it. When the run closes the outermost group, the argument
	// belongs to the group it just closed instead.
	enclosing := len(f.parens) - closes - 1
	if enclosing < 0 {
		enclosing = len(f.parens) - closes
	}
	restLevel = f.parens[enclosing].indent + 1
	return lines, rest, restLevel, lastLevel, closes, true
}
