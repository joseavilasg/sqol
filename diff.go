package main

import (
	"fmt"
	"strings"
)

// unifiedDiff renders the change between two texts as a patch: one hunk per run
// of changed lines, with three lines of context around it.
func unifiedDiff(before, after, name string) string {
	if before == after {
		return ""
	}
	ops := diffOps(lines(before), lines(after))

	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", name, name)
	for _, hunk := range hunks(ops, 3) {
		writeHunk(&out, ops, hunk[0], hunk[1])
	}
	return out.String()
}

// lines splits a document into its lines, dropping the newline that ends the
// last one: the diff prints it back.
func lines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// edit is one line of the change: kept, removed or added.
type edit struct {
	kind byte
	text string
}

// diffOps turns two documents into the shortest sequence of edits that carries
// one into the other, computed over their longest common subsequence.
func diffOps(before, after []string) []edit {
	n, m := len(before), len(after)
	common := make([][]int32, n+1)
	for i := range common {
		common[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case before[i] == after[j]:
				common[i][j] = common[i+1][j+1] + 1
			case common[i+1][j] >= common[i][j+1]:
				common[i][j] = common[i+1][j]
			default:
				common[i][j] = common[i][j+1]
			}
		}
	}

	ops := make([]edit, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case before[i] == after[j]:
			ops = append(ops, edit{' ', before[i]})
			i++
			j++
		case common[i+1][j] >= common[i][j+1]:
			ops = append(ops, edit{'-', before[i]})
			i++
		default:
			ops = append(ops, edit{'+', after[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, edit{'-', before[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, edit{'+', after[j]})
	}
	return ops
}

// hunks groups the edits into ranges of [from, to) that carry their changes
// plus the context around them. Two changes separated by fewer lines than the
// context takes to reach one another share a hunk.
func hunks(ops []edit, context int) [][2]int {
	var out [][2]int
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		first, last := i, i
		for i < len(ops) && (ops[i].kind != ' ' || i-last <= 2*context) {
			if ops[i].kind != ' ' {
				last = i
			}
			i++
		}
		from := max(first-context, 0)
		to := min(last+1+context, len(ops))
		out = append(out, [2]int{from, to})
	}
	return out
}

// writeHunk prints one hunk with its line numbers on each side.
func writeHunk(out *strings.Builder, ops []edit, from, to int) {
	oldLine, newLine := 1, 1
	for i := 0; i < from; i++ {
		switch ops[i].kind {
		case ' ':
			oldLine++
			newLine++
		case '-':
			oldLine++
		case '+':
			newLine++
		}
	}

	oldCount, newCount := 0, 0
	for i := from; i < to; i++ {
		switch ops[i].kind {
		case ' ':
			oldCount++
			newCount++
		case '-':
			oldCount++
		case '+':
			newCount++
		}
	}

	fmt.Fprintf(out, "@@ -%d,%d +%d,%d @@\n", oldLine, oldCount, newLine, newCount)
	for i := from; i < to; i++ {
		out.WriteByte(ops[i].kind)
		out.WriteString(ops[i].text)
		out.WriteByte('\n')
	}
}
