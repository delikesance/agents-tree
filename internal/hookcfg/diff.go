package hookcfg

import (
	"fmt"
	"strings"
)

const diffContext = 3

// Diff returns a unified-diff-like preview of old -> new ("" when equal). Inputs larger than
// maxDiffCells (lines x lines) get a coarse whole-file replacement instead of an LCS.
func Diff(oldName, newName, oldText, newText string) string {
	if oldText == newText {
		return ""
	}
	a, b := splitLines(oldText), splitLines(newText)
	type op struct {
		kind byte // ' ', '-', '+'
		text string
	}
	var ops []op
	if len(a)*len(b) > 4_000_000 {
		for _, l := range a {
			ops = append(ops, op{'-', l})
		}
		for _, l := range b {
			ops = append(ops, op{'+', l})
		}
	} else {
		// LCS table
		n, m := len(a), len(b)
		t := make([][]int32, n+1)
		for i := range t {
			t[i] = make([]int32, m+1)
		}
		for i := n - 1; i >= 0; i-- {
			for j := m - 1; j >= 0; j-- {
				if a[i] == b[j] {
					t[i][j] = t[i+1][j+1] + 1
				} else if t[i+1][j] >= t[i][j+1] {
					t[i][j] = t[i+1][j]
				} else {
					t[i][j] = t[i][j+1]
				}
			}
		}
		i, j := 0, 0
		for i < n || j < m {
			switch {
			case i < n && j < m && a[i] == b[j]:
				ops = append(ops, op{' ', a[i]})
				i++
				j++
			case j < m && (i == n || t[i][j+1] >= t[i+1][j]):
				ops = append(ops, op{'+', b[j]})
				j++
			default:
				ops = append(ops, op{'-', a[i]})
				i++
			}
		}
	}
	// keep ops within diffContext of a change
	keep := make([]bool, len(ops))
	for i, o := range ops {
		if o.kind == ' ' {
			continue
		}
		for k := max(0, i-diffContext); k <= min(len(ops)-1, i+diffContext); k++ {
			keep[k] = true
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", oldName, newName)
	oldLine, newLine := 1, 1
	for i := 0; i < len(ops); {
		if !keep[i] {
			if ops[i].kind != '+' {
				oldLine++
			}
			if ops[i].kind != '-' {
				newLine++
			}
			i++
			continue
		}
		j := i
		oc, nc := 0, 0
		for j < len(ops) && keep[j] {
			if ops[j].kind != '+' {
				oc++
			}
			if ops[j].kind != '-' {
				nc++
			}
			j++
		}
		os_, ns := oldLine, newLine
		if oc == 0 {
			os_--
		}
		if nc == 0 {
			ns--
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", os_, oc, ns, nc)
		for k := i; k < j; k++ {
			sb.WriteByte(ops[k].kind)
			sb.WriteString(ops[k].text)
			sb.WriteByte('\n')
		}
		oldLine += oc
		newLine += nc
		i = j
	}
	return sb.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
