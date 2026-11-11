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

	sqol -l .              # list the files whose layout would change
	sqol -w .              # rewrite them in place
	sqol -d .              # print the change as a unified diff
	sqol -stdin < q.sql    # format one document from standard input

A path argument is a file or a directory, and sqol walks a directory for its
`.sql` files. Without a path it walks the working directory, and without a flag
it prints the result to standard output. `-l` and `-d` change nothing and exit 1
when a file needs formatting, so a CI job can run either as a gate.

## What it does and does not do

SQoL reads the text, not a syntax tree. It works on the line structure you
already have: it does not split a statement written on one line into clauses,
and it does not reflow a query. What it does, it does the same way every time,
so formatting an already formatted file changes nothing. It makes no claim about
dialects: the rules apply to the keywords every dialect shares.

## License

[MIT](LICENSE)
