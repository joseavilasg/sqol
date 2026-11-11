# SQoL

**SQL Quality of Life.** An opinionated SQL formatter written in Go.

SQoL normalizes the layout of a query that is already split into clauses: it
re-indents a condition one level under the clause it belongs to, hangs a
parenthesis that spans lines with its contents below it, lines up the columns of
a `CREATE TABLE`, and uppercases keywords and built-in functions. String
literals and comments are left exactly as written.

Before:

```sql
SELECT id, title
FROM books
WHERE available
  AND (
      price < 10
  OR price IS NULL
  )
```

After:

```sql
SELECT id, title
FROM books
WHERE available
	AND (
		price < 10
		OR price IS NULL
	)
```

## Install

	go install github.com/joseavilasg/sqol@latest

The binary is `sqol`.

## Usage

	sqol -root ./migrations          # rewrite every .sql file under the path
	sqol -root ./migrations -check   # list what would change, exit 1 if any

The CI job runs `-check`: it changes nothing and exits 1 when a file needs
formatting.

## What it does and does not do

SQoL reads the text, not a syntax tree. It works on the line structure you
already have: it does not split a statement written on one line into clauses,
and it does not reflow a query. What it does, it does the same way every time,
so formatting an already formatted file changes nothing. It makes no claim about
dialects: the rules apply to the keywords every dialect shares.

## License

[MIT](LICENSE)
