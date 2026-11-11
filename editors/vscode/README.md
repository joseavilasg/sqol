# SQoL for Visual Studio Code

Formats a SQL file with [SQoL](https://github.com/joseavilasg/sqol) when you run
**Format Document** or save the file with format-on-save enabled.

The extension bundles a `sqol` binary for every platform it supports, so it
works without installing Go or anything else. It runs the binary for your
platform from the extension's own folder; on Windows it copies the binary out
first, because the system refuses to update an executable while a process runs
from it.

## Settings

| Setting     | Default | Meaning                                                                                   |
| ----------- | ------- | ----------------------------------------------------------------------------------------- |
| `sqol.path` | empty   | Path to a `sqol` binary. Empty uses the bundled one, then falls back to `sqol` on `PATH`. |

Set this to run a build of your own, for example while working on the
formatter.

## Scope

SQoL re-lays out SQL that already has its clauses on separate lines. It does not
split a statement written on one line, so format a document you have already
broken into clauses if it looks unchanged.
