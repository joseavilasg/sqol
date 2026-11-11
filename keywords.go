package main

import (
	"strings"
)

// keywords are written in upper case by convention. The list holds only words
// that cannot be mistaken for an identifier in this schema: a column called
// "type", "name", "size" or "value" is common, so those stay out and keep
// whatever case they were written in.
var keywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "AND": true, "OR": true,
	"NOT": true, "NULL": true, "IS": true, "IN": true, "AS": true, "ON": true,
	"USING": true, "JOIN": true, "LEFT": true, "RIGHT": true, "INNER": true,
	"FULL": true, "CROSS": true, "GROUP": true, "ORDER": true, "BY": true,
	"HAVING": true, "LIMIT": true, "OFFSET": true, "INSERT": true, "INTO": true,
	"VALUES": true, "UPDATE": true, "SET": true, "DELETE": true,
	"RETURNING": true, "WITH": true, "RECURSIVE": true, "UNION": true,
	"ALL": true, "DISTINCT": true, "CREATE": true, "ALTER": true, "DROP": true,
	"TABLE": true, "INDEX": true, "UNIQUE": true, "CONSTRAINT": true,
	"PRIMARY": true, "FOREIGN": true, "REFERENCES": true, "CHECK": true,
	"DEFAULT": true, "EXISTS": true, "CASE": true, "WHEN": true, "THEN": true,
	"ELSE": true, "END": true, "BEGIN": true, "DECLARE": true, "IF": true,
	"ELSIF": true, "LOOP": true, "DO": true, "CASCADE": true, "ASC": true,
	"DESC": true, "NULLS": true, "BETWEEN": true,
	"LIKE": true, "ILIKE": true, "CAST": true, "COLUMN": true, "ADD": true,
	"TRUE": true, "FALSE": true, "TO": true, "GRANT": true, "SCHEMA": true,
	"EXTENSION": true, "SEQUENCE": true, "TRIGGER": true, "FUNCTION": true,
	"LANGUAGE": true, "RAISE": true, "EXCEPTION": true, "NOTICE": true,
	"WARNING": true, "EXECUTE": true, "PERFORM": true, "STRICT": true,
	"VOLATILE": true, "STABLE": true, "IMMUTABLE": true, "SECURITY": true,
	"DEFINER": true, "FOR": true,
	// Type names, which read as part of the grammar rather than as data.
	"TEXT": true, "BIGINT": true, "INT": true, "INTEGER": true,
	"SMALLINT": true, "BOOLEAN": true, "TIMESTAMP": true, "DATE": true,
	"TIME": true, "INTERVAL": true, "NUMERIC": true, "DECIMAL": true,
	"REAL": true, "DOUBLE": true, "PRECISION": true, "VARCHAR": true,
	"CHARACTER": true, "VARYING": true, "JSONB": true, "JSON": true,
	"UUID": true, "BYTEA": true, "INET": true, "CIDR": true,
	"MACADDR": true, "SERIAL": true, "BIGSERIAL": true,
	"SMALLSERIAL": true, "ARRAY": true, "ENUM": true, "MONEY": true,
	"XML": true, "TSVECTOR": true, "TSQUERY": true, "HSTORE": true,
	"CITEXT": true,
}

// functions are the built-ins written in upper case, the way the standard and
// the manuals spell them. They only count when a parenthesis follows, so a
// column that shares the name stays as it was written.
var functions = map[string]bool{
	"COALESCE": true, "NULLIF": true, "GREATEST": true, "LEAST": true,
	"SUBSTRING": true, "TRIM": true, "POSITION": true, "OVERLAY": true,
	"EXTRACT": true, "CAST": true, "UPPER": true, "LOWER": true,
	"LENGTH": true, "ABS": true, "MOD": true, "COUNT": true, "SUM": true,
	"AVG": true, "MIN": true, "MAX": true, "CLOCK_TIMESTAMP": true,
	"RANDOM": true, "MD5": true, "NOW": true, "GENERATE_SERIES": true,
	"GENERATE_UID": true, "JSONB_AGG": true, "JSONB_ARRAY_ELEMENTS": true,
	"JSONB_BUILD_OBJECT": true, "JSONB_TO_RECORDSET": true,
	"PLAINTO_TSQUERY": true, "REGEXP_REPLACE": true, "TO_CHAR": true,
	"TO_TSVECTOR": true,
}

// uppercaseKeywords rewrites the keywords of a file in upper case. Everything
// that is not a bare word stays as written: text inside quotes, comments, and
// words joined to a dot, which belong to a qualified name.
func uppercaseKeywords(raw string) string {
	masked := mask(raw)
	var b strings.Builder
	b.Grow(len(raw))
	last := 0

	for i := 0; i < len(masked); i++ {
		if !isWordByte(masked[i]) {
			continue
		}
		start := i
		for i < len(masked) && isWordByte(masked[i]) {
			i++
		}
		word := raw[start:i]
		if i-start != len(word) {
			continue
		}
		upper := strings.ToUpper(word)
		if upper == word {
			continue
		}
		isCall := nextByte(masked, i) == '('
		switch {
		case isCall && functions[upper]:
		case !isCall && keywords[upper]:
		default:
			continue
		}
		// FIRST and LAST are only keywords right after NULLS; anywhere else they
		// can name an alias, and an alias keeps the case it was written in.
		if (upper == "FIRST" || upper == "LAST") && !strings.EqualFold(lastWord(masked, start), "NULLS") {
			continue
		}
		if prevByte(masked, start) == '.' || nextByte(masked, i) == '.' {
			continue
		}
		b.WriteString(raw[last:start])
		b.WriteString(upper)
		last = i
	}
	if last == 0 {
		return raw
	}
	b.WriteString(raw[last:])
	return b.String()
}

// lastWord returns the word that ends before start, skipping the spaces, or an
// empty string when the previous character is not a word.
func lastWord(s string, start int) string {
	i := start - 1
	for i >= 0 && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
		i--
	}
	end := i + 1
	for i >= 0 && isWordByte(s[i]) {
		i--
	}
	return s[i+1 : end]
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// prevByte returns the nearest byte before start that is not a space or a tab,
// or 0 at the beginning of the text.
func prevByte(s string, start int) byte {
	for i := start - 1; i >= 0; i-- {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			return s[i]
		}
	}
	return 0
}

// nextByte returns the nearest byte after end that is not a space or a tab, or
// 0 at the end of the text.
func nextByte(s string, end int) byte {
	for i := end; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && s[i] != '\r' {
			return s[i]
		}
	}
	return 0
}
