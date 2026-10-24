package migrate

import (
	"fmt"
	"path/filepath"
	"strings"
)

// diffContext is the number of unchanged lines around each hunk.
const diffContext = 3

// UnifiedDiff is the unified diff of old and new, labelled a/name and
// b/name (/dev/null for a missing old file); "" when they are equal.
func UnifiedDiff(name string, old, new []byte) string {
	if string(old) == string(new) {
		return ""
	}
	a, b := splitLines(string(old)), splitLines(string(new))
	ops := diffLines(a, b)
	var out strings.Builder
	from, to := "a/"+name, "b/"+name
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		from, to = name, name
	}
	if old == nil {
		from = "/dev/null"
	}
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", from, to)
	for start := 0; start < len(ops); {
		// Find the next change.
		for start < len(ops) && ops[start].kind == ' ' {
			start++
		}
		if start == len(ops) {
			break
		}
		lo := max(start-diffContext, 0)
		hi := start
		// Extend the hunk while changes are closer than two contexts.
		for i := start; i < len(ops); i++ {
			if ops[i].kind != ' ' {
				hi = i
				continue
			}
			if i-hi > 2*diffContext {
				break
			}
		}
		hi = min(hi+diffContext+1, len(ops))
		aStart, bStart, aLen, bLen := 0, 0, 0, 0
		for i := 0; i < lo; i++ {
			if ops[i].kind != '+' {
				aStart++
			}
			if ops[i].kind != '-' {
				bStart++
			}
		}
		for i := lo; i < hi; i++ {
			if ops[i].kind != '+' {
				aLen++
			}
			if ops[i].kind != '-' {
				bLen++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(aStart, aLen), hunkRange(bStart, bLen))
		for i := lo; i < hi; i++ {
			fmt.Fprintf(&out, "%c%s\n", ops[i].kind, ops[i].line)
		}
		start = hi
	}
	return out.String()
}

func hunkRange(start, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if n == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	return strings.Split(s, "\n")
}

type diffOp struct {
	kind byte // ' ', '-' or '+'
	line string
}

// diffLines is a longest-common-subsequence line diff: the files nerv
// migrate writes are a few hundred lines at most.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
