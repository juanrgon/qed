// Package classify enumerates small strength-2 covering arrays
// CA(N; 2, k, v) exhaustively and sorts them into isomorphism classes.
//
// Two arrays are isomorphic if one becomes the other by permuting rows,
// permuting columns, and permuting the symbols inside each column.
//
// Levels adds one column at a time and keeps one representative per
// class at each step (see Levels for why this misses nothing).
//
// Which way errors can go. Two arrays are only merged into one class when
// FindIso returns a map and Apply plus a plain comparison of sorted rows
// confirms it, so a bug there cannot merge distinct classes. A bug that
// misses an isomorphism only adds extra classes. So a count of 1 (or 0)
// can only be too small if Extensions misses a column, and Extensions is
// a plain search whose only pruning is "more pairs missing than rows
// left".
package classify

import (
	"slices"
	"sort"
)

// Array is a column-major N by k array: A[c][r].
type Array [][]uint8

// FromRows builds an Array from row-major data.
func FromRows(rows [][]uint8) Array {
	if len(rows) == 0 {
		return nil
	}
	k := len(rows[0])
	a := make(Array, k)
	for c := range a {
		a[c] = make([]uint8, len(rows))
		for r := range rows {
			a[c][r] = rows[r][c]
		}
	}
	return a
}

func (a Array) N() int { return len(a[0]) }
func (a Array) K() int { return len(a) }

// IsCA reports whether a covers every pair of symbols on every pair of
// columns. It is a plain check, independent of the search.
func IsCA(a Array, v int) bool {
	for c1 := 0; c1 < a.K(); c1++ {
		for c2 := c1 + 1; c2 < a.K(); c2++ {
			seen := make([]bool, v*v)
			for r := 0; r < a.N(); r++ {
				seen[int(a[c1][r])*v+int(a[c2][r])] = true
			}
			for _, s := range seen {
				if !s {
					return false
				}
			}
		}
	}
	return true
}

// Iso maps an array X onto an array Y: column c of X goes to column
// Col[c] of Y, with symbol s replaced by Sym[c][s].
type Iso struct {
	Col []int
	Sym [][]uint8
}

// Apply returns X relabeled by m, with columns in Y's order.
func Apply(x Array, m Iso) Array {
	y := make(Array, x.K())
	for c := range x {
		col := make([]uint8, x.N())
		for r, s := range x[c] {
			col[r] = m.Sym[c][s]
		}
		y[m.Col[c]] = col
	}
	return y
}

// SortedRows returns the rows of a as strings in sorted order, so two
// arrays are equal up to row order exactly when these are equal.
func SortedRows(a Array) []string {
	rows := make([]string, a.N())
	for r := range rows {
		b := make([]byte, a.K())
		for c := range a {
			b[c] = a[c][r]
		}
		rows[r] = string(b)
	}
	sort.Strings(rows)
	return rows
}

// SameUpToRows reports whether a and b have the same multiset of rows.
func SameUpToRows(a, b Array) bool {
	return a.K() == b.K() && a.N() == b.N() && slices.Equal(SortedRows(a), SortedRows(b))
}

func perms(v int) [][]uint8 {
	var out [][]uint8
	p := make([]uint8, v)
	used := make([]bool, v)
	var rec func(i int)
	rec = func(i int) {
		if i == v {
			out = append(out, slices.Clone(p))
			return
		}
		for s := 0; s < v; s++ {
			if !used[s] {
				used[s] = true
				p[i] = uint8(s)
				rec(i + 1)
				used[s] = false
			}
		}
	}
	rec(0)
	return out
}

// FindIso searches for an isomorphism from x onto y. It maps x's columns
// one at a time and prunes as soon as the partial rows of x and y differ
// as multisets.
func FindIso(x, y Array, v int) (Iso, bool) {
	k, n := x.K(), x.N()
	if y.K() != k || y.N() != n {
		return Iso{}, false
	}
	ps := perms(v)
	m := Iso{Col: make([]int, k), Sym: make([][]uint8, k)}
	used := make([]bool, k)
	xc := make([][]uint64, k+1) // partial row codes after c mapped columns
	yc := make([][]uint64, k+1)
	xc[0] = make([]uint64, n)
	yc[0] = make([]uint64, n)
	sx := make([]uint64, n)
	sy := make([]uint64, n)
	var rec func(c int) bool
	rec = func(c int) bool {
		if c == k {
			return true
		}
		for d := 0; d < k; d++ {
			if used[d] {
				continue
			}
			for _, p := range ps {
				xn := make([]uint64, n)
				yn := make([]uint64, n)
				for r := 0; r < n; r++ {
					xn[r] = xc[c][r]*uint64(v) + uint64(p[x[c][r]])
					yn[r] = yc[c][r]*uint64(v) + uint64(y[d][r])
				}
				copy(sx, xn)
				copy(sy, yn)
				slices.Sort(sx)
				slices.Sort(sy)
				if !slices.Equal(sx, sy) {
					continue
				}
				used[d] = true
				m.Col[c] = d
				m.Sym[c] = p
				xc[c+1], yc[c+1] = xn, yn
				if rec(c + 1) {
					return true
				}
				used[d] = false
			}
		}
		return false
	}
	if !rec(0) {
		return Iso{}, false
	}
	return m, true
}

// Levels classifies CA(n; 2, j, v) for j = 1..k by adding one column at a
// time. levels[j-1] holds one representative per isomorphism class of
// arrays with j columns that cover every pair (for j = 1: every symbol
// appears at least v times, which any column of a 2-column CA needs).
//
// Completeness. Take any CA with j+1 columns. Its first j columns are
// isomorphic to some level-j representative R; apply that isomorphism
// (it permutes rows, and relabels symbols in those columns) to the whole
// array. Now the first j columns equal R row for row, and the last column
// is some vector. Relabel its symbols so they first appear in the order
// 0, 1, 2. Extensions lists exactly the vectors of that form that cover
// every pair with R's columns, so the array is found.
func Levels(n, k, v int) [][]Array {
	var levels [][]Array
	// Level 1: one column, symbols sorted, counts nonincreasing.
	var first []Array
	var rec func(sym, left, prev int, counts []int)
	rec = func(sym, left, prev int, counts []int) {
		if sym == v {
			if left == 0 {
				col := make([]uint8, 0, n)
				for s, c := range counts {
					for i := 0; i < c; i++ {
						col = append(col, uint8(s))
					}
				}
				first = append(first, Array{col})
			}
			return
		}
		for c := v; c <= left && c <= prev; c++ {
			rec(sym+1, left-c, c, append(counts, c))
		}
	}
	rec(0, n, n, nil)
	levels = append(levels, first)
	for j := 1; j < k; j++ {
		var cands []Array
		for _, r := range levels[j-1] {
			for _, col := range Extensions(r, v) {
				a := make(Array, j+1)
				copy(a, r)
				a[j] = col
				cands = append(cands, a)
			}
		}
		var reps []Array
		for _, c := range Classify(cands, v) {
			reps = append(reps, c.Rep)
		}
		levels = append(levels, reps)
	}
	return levels
}

// Extensions lists every new column for r whose symbols first appear in
// the order 0, 1, 2, ... and that covers every pair with each column of r.
func Extensions(r Array, v int) [][]uint8 {
	n, k := r.N(), r.K()
	vv := v * v
	cov := make([][]int, k)
	miss := make([]int, k)
	for c := range cov {
		cov[c] = make([]int, vv)
		miss[c] = vv
	}
	col := make([]uint8, n)
	var out [][]uint8
	var fill func(i, maxUsed int)
	fill = func(i, maxUsed int) {
		if i == n {
			for c := 0; c < k; c++ {
				if miss[c] != 0 {
					return
				}
			}
			out = append(out, slices.Clone(col))
			return
		}
		left := n - i - 1
		hi := maxUsed + 1
		if hi > v-1 {
			hi = v - 1
		}
		for x := 0; x <= hi; x++ {
			col[i] = uint8(x)
			ok := true
			for c := 0; c < k; c++ {
				p := int(r[c][i])*v + x
				if cov[c][p] == 0 {
					miss[c]--
				}
				cov[c][p]++
				if miss[c] > left {
					ok = false
				}
			}
			if ok {
				m := maxUsed
				if x > m {
					m = x
				}
				fill(i+1, m)
			}
			for c := 0; c < k; c++ {
				p := int(r[c][i])*v + x
				cov[c][p]--
				if cov[c][p] == 0 {
					miss[c]++
				}
			}
		}
	}
	fill(0, -1)
	return out
}

// invariant is unchanged by row, column and symbol permutations: the
// sorted symbol counts of each column, then for each row the histogram of
// how many columns it agrees on with each other row, as a sorted list.
func invariant(a Array, v int) string {
	var cols []string
	for c := range a {
		cnt := make([]int, v)
		for _, s := range a[c] {
			cnt[s]++
		}
		slices.Sort(cnt)
		b := make([]byte, v)
		for i, x := range cnt {
			b[i] = byte(x)
		}
		cols = append(cols, string(b))
	}
	sort.Strings(cols)
	rows := make([]string, a.N())
	for r1 := range rows {
		hist := make([]byte, a.K()+1)
		for r2 := 0; r2 < a.N(); r2++ {
			if r2 == r1 {
				continue
			}
			agree := 0
			for c := range a {
				if a[c][r1] == a[c][r2] {
					agree++
				}
			}
			hist[agree]++
		}
		rows[r1] = string(hist)
	}
	sort.Strings(rows)
	key := ""
	for _, s := range cols {
		key += s + "|"
	}
	for _, s := range rows {
		key += s + "|"
	}
	return key
}

// Class is one isomorphism class: a representative and how many
// normal-form arrays the search found in it.
type Class struct {
	Rep   Array
	Found int
}

// Classify sorts arrays into isomorphism classes. Every assignment is
// backed by an explicit isomorphism that is checked with Apply and
// SameUpToRows, so a bug in FindIso cannot merge two classes silently.
func Classify(arrays []Array, v int) []Class {
	var classes []Class
	byInv := map[string][]int{}
	for _, a := range arrays {
		inv := invariant(a, v)
		placed := false
		for _, ci := range byInv[inv] {
			if m, ok := FindIso(a, classes[ci].Rep, v); ok {
				if !SameUpToRows(Apply(a, m), classes[ci].Rep) {
					panic("classify: isomorphism failed its check")
				}
				classes[ci].Found++
				placed = true
				break
			}
		}
		if !placed {
			byInv[inv] = append(byInv[inv], len(classes))
			classes = append(classes, Class{Rep: a, Found: 1})
		}
	}
	return classes
}
