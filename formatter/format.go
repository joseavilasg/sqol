// Package formatter rewrites SQL with tabs, one clause per line and one
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
package formatter

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	keywordRe = regexp.MustCompile(`^(SELECT|FROM|WHERE|GROUP\s+BY|ORDER\s+BY|HAVING|LIMIT|OFFSET|RETURNING|VALUES|SET|UNION|EXCEPT|INTERSECT|INSERT|UPDATE|DELETE|WITH|JOIN|LEFT|RIGHT|INNER|FULL|CROSS|ON|USING|WINDOW)\b`)
	starterRe = regexp.MustCompile(`^(SELECT|INSERT|UPDATE|DELETE|WITH)\b`)
	selectRe  = regexp.MustCompile(`^SELECT\b`)
	unionRe   = regexp.MustCompile(`\bUNION(\s+ALL)?\b`)
)

// Settle applies the rewrite until the text stops changing. One pass is enough
// when the file already follows the shape; a file written by an older version of
// this package can need a second, because a continuation sits relative to the
// line that started its statement and that line moves in the first pass.
func Settle(raw string) (string, error) {
	current := raw
	for range 20 {
		next := format(current)
		if next == current {
			return next, nil
		}
		current = next
	}
	return current, fmt.Errorf("the layout does not settle after twenty passes")
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
// mask blanks quoted text, quoted identifiers and comments. The result keeps the
// length of its input so positions line up.
func mask(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		i += maskOne(s[i:], &b)
	}
	return b.String()
}

// maskOne writes the blanked form of the construct at the front of s and returns
// how many bytes it consumed.
func maskOne(s string, b *strings.Builder) int {
	switch {
	case strings.HasPrefix(s, "--"):
		return maskLineComment(s, b)
	case strings.HasPrefix(s, "/*"):
		return maskBlockComment(s, b)
	case s[0] == '\'':
		return maskSingleQuoted(s, b)
	case s[0] == '"':
		return maskDoubleQuoted(s, b)
	default:
		b.WriteByte(s[0])
		return 1
	}
}

// maskLineComment blanks a comment that ends at the newline, which it keeps.
func maskLineComment(s string, b *strings.Builder) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			b.WriteByte('\n')
			return i + 1
		}
		b.WriteByte(' ')
	}
	return len(s)
}

// maskBlockComment blanks a comment up to and including its closing delimiter.
func maskBlockComment(s string, b *strings.Builder) int {
	b.WriteString("  ")
	for i := 2; i < len(s); i++ {
		b.WriteByte(' ')
		if s[i] == '*' && i+1 < len(s) && s[i+1] == '/' {
			b.WriteByte(' ')
			return i + 2
		}
	}
	return len(s)
}

// maskSingleQuoted blanks a literal, where a doubled quote stands for itself.
func maskSingleQuoted(s string, b *strings.Builder) int {
	b.WriteByte(' ')
	for i := 1; i < len(s); i++ {
		b.WriteByte(' ')
		if s[i] != '\'' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '\'' {
			b.WriteByte(' ')
			i++
			continue
		}
		return i + 1
	}
	return len(s)
}

// maskDoubleQuoted blanks a quoted identifier.
func maskDoubleQuoted(s string, b *strings.Builder) int {
	b.WriteByte(' ')
	for i := 1; i < len(s); i++ {
		b.WriteByte(' ')
		if s[i] == '"' {
			return i + 1
		}
	}
	return len(s)
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

// format rewrites one document in a single pass.
func format(raw string) string {
	lines := prepare(raw)
	r := &renderer{f: &formatter{fresh: true}}
	for _, ln := range lines {
		r.line(ln)
	}
	return r.text()
}

// prepare uppercases the keywords and lines up the columns of every table. The
// widths a table shares depend on every line of its body, so this step cannot
// run one line at a time; the text is read back afterwards so every line carries
// its masked view.
func prepare(raw string) []source {
	aligned := alignColumns(readLines(uppercaseKeywords(raw)))
	text := make([]string, len(aligned))
	for i := range aligned {
		text[i] = aligned[i].text
	}
	return readLines(strings.Join(text, "\n"))
}

// renderer builds the output of one pass. It carries the formatter because every
// line can change the state the next one reads.
type renderer struct {
	f        *formatter
	out      []string
	lastCode string
}

func (r *renderer) line(ln source) {
	switch {
	case ln.blank:
		r.blank()
	case ln.comment:
		r.comment(ln)
	default:
		r.code(ln)
	}
}

func (r *renderer) blank() {
	r.out = append(r.out, "")
	r.f.fresh = true
	r.f.inClause = false
	r.f.afterUnion = false
	r.lastCode = ""
}

// comment keeps the level of the block it sits in: flattening it to the left
// margin detaches it from the lines it explains. A goose annotation is refused
// with any leading whitespace, so it always starts at the margin.
func (r *renderer) comment(ln source) {
	level := 0
	switch {
	case strings.HasPrefix(ln.trim, "-- +goose"):
	case len(r.f.blocks) > 0 || len(r.f.parens) > 0:
		level = r.f.base()
	case strings.HasSuffix(r.lastCode, ","):
		level = r.f.prevInd
	}
	r.out = append(r.out, tabs(level)+ln.trim)
}

func (r *renderer) code(ln source) {
	trim := ln.trim
	upper := strings.ToUpper(trim)
	isUnion := r.unionSpacing(upper)

	if r.closeRun(trim) {
		return
	}
	cur := r.level(trim, upper)
	if r.openingGroup(trim, cur) || r.inlineClose(trim, cur) || r.inlineUnion(trim, cur, ln) {
		return
	}
	if r.selectList(ln, trim, cur) {
		return
	}
	r.plain(ln, trim, cur, isUnion)
}

// unionSpacing puts a blank line on each side of a UNION: it separates two
// result sets, and without it the seam reads as another clause.
func (r *renderer) unionSpacing(upper string) bool {
	if r.f.afterUnion {
		r.out = append(r.out, "")
		r.f.afterUnion = false
	}
	isUnion := upper == "UNION" || strings.HasPrefix(upper, "UNION ALL") || strings.HasPrefix(upper, "UNION ")
	if isUnion && r.blankBefore() {
		r.out = append(r.out, "")
	}
	return isUnion
}

func (r *renderer) blankBefore() bool {
	return len(r.out) > 0 && r.out[len(r.out)-1] != ""
}

// closeRun handles a line that closes groups opened on earlier lines: one line
// per group, and an argument after the run starts below the call it belongs to.
// A lone closing with nothing after it is left to the accounting.
func (r *renderer) closeRun(trim string) bool {
	lines, rest, restLevel, lastLevel, closes, ok := r.f.closeLines(trim)
	if !ok {
		return false
	}
	r.out = append(r.out, lines...)
	r.f.closeParens(closes)
	if rest == "" {
		r.prev(lastLevel, ")", false)
		r.f.prevClose = true
		r.lastCode = ")"
		return true
	}
	r.out = append(r.out, tabs(restLevel)+rest)
	r.prev(restLevel, rest, false)
	r.f.prevClose = false
	r.lastCode = rest
	r.f.update(mask(rest), 0, restLevel)
	return true
}

func (r *renderer) prev(level int, text string, clause bool) {
	r.f.prevInd, r.f.prevTrim, r.f.prevClause = level, text, clause
}

func (r *renderer) level(trim, upper string) int {
	if r.f.inBody {
		return r.f.body(trim, upper)
	}
	return r.f.statement(trim, upper)
}

// openingGroup ends the line with a parenthesis that opens a block, so the group
// reads as one and its contents sit one level below.
func (r *renderer) openingGroup(trim string, cur int) bool {
	head, rest, ok := openSplit(trim)
	if !ok {
		return false
	}
	r.out = append(r.out, tabs(cur)+head, tabs(cur+1)+rest)
	r.f.open(cur)
	r.f.update(mask(rest), 0, cur+1)
	r.prev(cur+1, rest, false)
	r.lastCode = rest
	return true
}

// inlineClose gives a closing parenthesis that shares its line with the
// statement inside it a line of its own, so the group reads as a block. Only a
// parenthesis opened on an earlier line counts; one that opens and closes here
// belongs to the expression.
func (r *renderer) inlineClose(trim string, cur int) bool {
	prefix, suffix, opener, ok := r.f.closeSplitAt(trim, len(r.f.parens))
	if !ok {
		return false
	}
	r.out = append(r.out, tabs(cur)+prefix, tabs(opener)+suffix)
	r.f.update(mask(prefix), 0, cur)
	// The suffix starts with that parenthesis, so it is popped here instead of
	// being left to the stack's own accounting.
	r.f.drop()
	r.f.update(mask(suffix), leadingCloses(suffix), opener)
	r.prev(opener, suffix, false)
	r.lastCode = suffix
	return true
}

// inlineUnion splits a UNION that shares its line with both queries, so the
// blank lines that set it apart have somewhere to go. When the union sits inside
// parentheses that open on the same line, they take their own lines too;
// otherwise the closing one would trail the last select.
func (r *renderer) inlineUnion(trim string, cur int, ln source) bool {
	pre, parts, post, ok := splitUnionLine(trim)
	if !ok {
		return false
	}
	if pre == "" {
		for _, part := range parts {
			r.unionLine(cur, part)
		}
		r.f.update(ln.masked, leadingCloses(trim), cur)
	} else {
		r.emit(cur, pre)
		r.openAll(mask(pre), cur)
		for _, part := range parts {
			r.unionLine(cur+1, part)
			r.trackParens(mask(part), cur+1)
		}
		r.emitPost(post, cur)
	}
	r.prev(cur, trim, false)
	r.lastCode = trim
	return true
}

func (r *renderer) emit(level int, text string) {
	r.out = append(r.out, tabs(level)+text)
}

// unionLine writes one branch, separated from the keyword by a blank line.
func (r *renderer) unionLine(level int, text string) {
	keyword := isUnionPart(text)
	if r.f.afterUnion {
		r.out = append(r.out, "")
		r.f.afterUnion = false
	}
	if keyword && r.blankBefore() {
		r.out = append(r.out, "")
	}
	r.emit(level, text)
	r.f.afterUnion = keyword
}

func (r *renderer) emitPost(post string, cur int) {
	if post == "" {
		return
	}
	if r.f.afterUnion {
		r.out = append(r.out, "")
		r.f.afterUnion = false
	}
	r.emit(cur, post)
	r.trackParens(mask(post), cur)
}

func (r *renderer) openAll(masked string, level int) {
	for _, c := range masked {
		if c == '(' {
			r.f.open(level)
		}
	}
}

func (r *renderer) trackParens(masked string, level int) {
	for _, c := range masked {
		switch c {
		case '(':
			r.f.open(level)
		case ')':
			r.f.drop()
		}
	}
}

// selectList moves the list of a SELECT that continues on the next line under
// the keyword.
func (r *renderer) selectList(ln source, trim string, cur int) bool {
	if !selectRe.MatchString(trim) || !strings.HasSuffix(trim, ",") {
		return false
	}
	head, rest := splitSelect(trim)
	r.out = append(r.out, tabs(cur)+head)
	if rest != "" {
		r.out = append(r.out, tabs(cur+1)+rest)
	}
	r.prev(cur+1, rest, false)
	headMask, restMask := splitSelect(ln.masked)
	r.f.update(headMask, 0, cur)
	r.f.update(restMask, 0, cur+1)
	return true
}

// plain writes a line that needs no reshaping and leaves the state the next one
// reads.
func (r *renderer) plain(ln source, trim string, cur int, isUnion bool) {
	r.out = append(r.out, tabs(cur)+trim)
	r.lastCode = trim
	r.f.afterUnion = isUnion
	r.f.prevInd, r.f.prevTrim = cur, trim
	r.f.prevClause = keywordRe.MatchString(trim)
	closes := leadingCloses(trim)
	if closes == 0 {
		r.f.prevClose = false
	}
	r.f.update(ln.masked, closes, cur)
	// A semicolon ends the statement, so the next line starts a new one. The rule
	// matters inside a DECLARE block, where every variable is its own statement
	// and none of them is a keyword.
	if strings.HasSuffix(trim, ";") {
		r.f.fresh = true
	}
	if !r.f.inBody && strings.HasSuffix(ln.masked, "$$") {
		r.f.inBody = true
		r.f.fresh = true
	}
}

func (r *renderer) text() string {
	for len(r.out) > 1 && r.out[len(r.out)-1] == "" && r.out[len(r.out)-2] == "" {
		r.out = r.out[:len(r.out)-1]
	}
	return strings.Join(r.out, "\n")
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

// drop closes the innermost parenthesis without reporting where it sat.
func (f *formatter) drop() {
	if len(f.parens) > 0 {
		f.parens = f.parens[:len(f.parens)-1]
	}
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
	if open, shut, found := enclosingParen(masked, matches[0][0], matches[len(matches)-1][1]); found {
		pre, body, post = trim[:open+1], trim[open+1:shut], trim[shut:]
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
func enclosingParen(masked string, first, last int) (open, shut int, ok bool) {
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
				best, shut = o, i
			}
		}
	}
	if best == -1 {
		return 0, 0, false
	}
	return best, shut, true
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
