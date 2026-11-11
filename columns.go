package main

import (
	"strings"
)

// column is one line inside a CREATE TABLE body, split into the cells that line
// up: the name, its type, whether it accepts NULL, and whatever follows.
type column struct {
	name  string
	typ   string
	null  string
	rest  string
	raw   string // the line as written, when it does not look like a column
	isCol bool
}

// constraintWords end the type of a column. What comes after them is a
// constraint, not part of the type.
var constraintWords = map[string]bool{
	"NOT": true, "NULL": true, "DEFAULT": true, "PRIMARY": true,
	"REFERENCES": true, "UNIQUE": true, "CHECK": true, "CONSTRAINT": true,
	"COLLATE": true, "GENERATED": true,
}

// alignColumns lines up the columns of every CREATE TABLE in the file. The
// longest name and the longest type set the width of their cells, so a reader
// scans one column at a time instead of reading a ragged list.
func alignColumns(lines []source) []source {
	for i := 0; i < len(lines); i++ {
		if !opensTable(lines[i]) {
			continue
		}
		end := tableEnd(lines, i+1)
		if end <= i+1 {
			continue
		}
		alignRange(lines, i+1, end)
		i = end
	}
	return lines
}

// opensTable reports whether the line starts a CREATE TABLE whose body follows.
func opensTable(ln source) bool {
	upper := strings.ToUpper(strings.TrimSpace(ln.masked))
	return strings.HasPrefix(upper, "CREATE TABLE") && strings.HasSuffix(upper, "(")
}

// tableEnd returns the index of the line that closes the body opened before
// start, or start when it does not close at all.
func tableEnd(lines []source, start int) int {
	depth := 1
	for i := start; i < len(lines); i++ {
		if lines[i].blank || lines[i].comment {
			continue
		}
		for _, c := range lines[i].masked {
			switch c {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return start
}

// alignRange rewrites the column lines between start and end inclusive of the
// last column line, leaving comments and blanks where they are.
func alignRange(lines []source, start, end int) {
	var cols []column
	for i := start; i < end; i++ {
		ln := lines[i]
		if ln.blank || ln.comment {
			continue
		}
		cols = append(cols, parseColumn(ln.trim))
	}
	if len(cols) == 0 {
		return
	}

	maxName, maxType, hasNull := 0, 0, false
	for _, c := range cols {
		if !c.isCol {
			continue
		}
		maxName = max(maxName, len(c.name))
		maxType = max(maxType, len(c.typ))
		if c.null != "" {
			hasNull = true
		}
	}
	if maxName == 0 {
		return
	}

	index := 0
	for i := start; i < end; i++ {
		ln := lines[i]
		if ln.blank || ln.comment {
			continue
		}
		c := cols[index]
		index++
		if !c.isCol {
			lines[i].text = c.raw
			lines[i].trim = c.raw
			continue
		}
		var b strings.Builder
		b.WriteString(c.name)
		b.WriteString(strings.Repeat(" ", maxName-len(c.name)+1))
		b.WriteString(c.typ)
		if c.null != "" || hasNull {
			b.WriteString(strings.Repeat(" ", maxType-len(c.typ)+1))
			if c.null != "" {
				b.WriteString("NOT NULL")
			} else {
				b.WriteString(strings.Repeat(" ", len("NOT NULL")))
			}
		}
		if c.rest != "" {
			if c.rest == "," {
				b.WriteString(",")
			} else {
				b.WriteString(" ")
				b.WriteString(c.rest)
			}
		}
		lines[i].text = b.String()
		lines[i].trim = b.String()
	}
}

// parseColumn splits a line of a table body into its cells. A line that starts
// with a table constraint keeps its text and is not aligned as a column.
func parseColumn(trim string) column {
	if trim == "" {
		return column{}
	}
	upper := strings.ToUpper(trim)
	for _, prefix := range []string{"CONSTRAINT ", "PRIMARY KEY", "UNIQUE", "CHECK", "FOREIGN KEY", "EXCLUDE"} {
		if strings.HasPrefix(upper, prefix) {
			return column{raw: trim}
		}
	}

	tokens := splitTokens(trim)
	if len(tokens) < 2 {
		return column{raw: trim}
	}
	c := column{name: tokens[0], isCol: true}
	i := 1
	for ; i < len(tokens); i++ {
		if constraintWords[strings.ToUpper(tokens[i])] {
			break
		}
		if c.typ == "" {
			c.typ = tokens[i]
		} else {
			c.typ += " " + tokens[i]
		}
	}
	if c.typ == "" {
		return column{raw: trim}
	}
	rest := strings.Join(tokens[i:], " ")
	if strings.HasPrefix(strings.ToUpper(rest), "NOT NULL") {
		c.null = "NOT NULL"
		rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), "NOT NULL"))
	}
	c.rest = strings.ReplaceAll(rest, " ,", ",")
	return c
}

// splitTokens cuts a line at whitespace outside quotes, so a default that holds
// a phrase stays one token.
func splitTokens(s string) []string {
	var tokens []string
	var b strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			b.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			if b.Len() > 0 {
				tokens = append(tokens, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		tokens = append(tokens, b.String())
	}
	return tokens
}
