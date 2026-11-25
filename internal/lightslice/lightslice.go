// Package lightslice checks the "light slice" argument for lower bounds on
// CAN(3, 8, 3), using only the definition of a covering array.
//
// Setup. In a CA(N; 3, 8, 3), the rows with column j equal to s (a
// "slice") must cover every pair on the other 7 columns, so a slice is a
// CA(n; 2, 7, 3) and has n >= 12 rows. Call s "light" in column j if
// exactly 12 rows have it. If N <= 38, every column has a light symbol.
//
// Pin column 0's light slice A (symbol 0) to a given 12-row array. For a
// column Y and symbol v, the light slice B = {rows with Y = v} must:
//   - contain exactly the rows of A with Y = v (every row with column
//     0 equal to 0 is in A), and no other row with column 0 equal to 0;
//   - cover every pair on the 7 columns other than Y with 12 rows.
//
// Completions enumerates every such B directly by search. It does not use
// the uniqueness of CA(12; 2, 7, 3), any balance property, or any
// isomorphism code.
//
// Two light slices B (Y = v) and C (Z = w) share exactly the array rows
// with Y = v and Z = w, so {b in B : b[Z] = w} and {c in C : c[Y] = v}
// must be equal as multisets. If for some columns Y, Z no pair (B, C) is
// compatible for any of the 9 choices of (v, w), then no CA(N; 3, 8, 3)
// with N <= 38 has column 0's light slice equal to A.
//
// Turning that into "CAN(3, 8, 3) >= 39" still needs one outside fact:
// CA(12; 2, 7, 3) is unique up to isomorphism, so A can be pinned.
package lightslice

import (
	"slices"
	"sync"

	"github.com/juanrgon/mathbench/internal/ca"
)

// MaxK is the most columns a packed Row holds. The main case is K = 8
// with 12-row slices; the test suite also runs K = 6 with 11-row slices,
// where a real array (the unique CA(33; 3, 6, 3)) exists, as a control.
const MaxK = 8

// Row is a row of up to 8 columns packed in base 3: digit c is column c.
type Row uint16

var pow3 = [MaxK + 1]int{1, 3, 9, 27, 81, 243, 729, 2187, 6561}

func (r Row) At(c int) uint8 { return uint8(int(r) / pow3[c] % 3) }

func pack(vals []uint8) Row {
	x := 0
	for c := len(vals) - 1; c >= 0; c-- {
		x = x*3 + int(vals[c])
	}
	return Row(x)
}

// Pinned builds A: column 0 is 0 and columns 1.. are the given slice.
func Pinned(r *ca.Array) []Row {
	if r.K+1 > MaxK {
		panic("too many columns")
	}
	out := make([]Row, r.N)
	for i := 0; i < r.N; i++ {
		vals := append([]uint8{0}, r.Row(i)...)
		out[i] = pack(vals)
	}
	return out
}

// Slice is one candidate light slice: 12 rows, sorted.
type Slice []Row

// Completions returns every multiset of 12 rows with column Y equal to v
// that contains exactly A's rows with Y = v, whose other rows have column
// 0 in {1, 2}, and that covers all pairs on the columns other than Y.
//
// It fills the new rows one column at a time. Rows are kept in
// lexicographic order (a row may not be smaller than the row above it if
// they agree on every earlier column), so each multiset is built exactly
// once. After each column, every earlier column must show all 9 pairs
// with it; while a column is half filled, each earlier column may have at
// most as many missing pairs as there are rows left to fill.
//
// k is the number of columns in the big array and size the slice size
// (8 and 12 for the main result).
func Completions(A []Row, k, size, Y int, v uint8) []Slice {
	var fixed []Row
	for _, r := range A {
		if r.At(Y) == v {
			fixed = append(fixed, r)
		}
	}
	need := size - len(fixed)
	if need <= 0 {
		return nil
	}
	cols := []int{0}
	for c := 1; c < k; c++ {
		if c != Y {
			cols = append(cols, c)
		}
	}
	nc := len(cols)
	M := make([][]uint8, need) // M[i][j]: new row i at cols[j]
	for i := range M {
		M[i] = make([]uint8, nc)
	}
	var cov [MaxK][9]int // coverage of (cols[jp], cols[j]) for the column being filled
	var miss [MaxK]int

	seen := map[string]bool{}
	var out []Slice
	emit := func() {
		s := append(Slice{}, fixed...)
		for i := 0; i < need; i++ {
			vals := make([]uint8, k)
			vals[Y] = v
			for j, c := range cols {
				vals[c] = M[i][j]
			}
			s = append(s, pack(vals))
		}
		slices.Sort(s)
		key := string(rowBytes(s))
		if seen[key] {
			panic("lightslice: duplicate completion; ordering logic is wrong")
		}
		seen[key] = true
		out = append(out, s)
	}

	var fill func(j, i int)
	startColumn := func(j int) bool {
		c := cols[j]
		for jp := 0; jp < j; jp++ {
			cov[jp] = [9]int{}
			miss[jp] = 9
			for _, r := range fixed {
				x := int(r.At(cols[jp]))*3 + int(r.At(c))
				if cov[jp][x] == 0 {
					miss[jp]--
				}
				cov[jp][x]++
			}
			if miss[jp] > need {
				return false
			}
		}
		return true
	}
	fill = func(j, i int) {
		if j == nc {
			emit()
			return
		}
		if i == need {
			for jp := 0; jp < j; jp++ {
				if miss[jp] != 0 {
					return
				}
			}
			if j+1 == nc {
				fill(j+1, 0)
				return
			}
			if startColumn(j + 1) {
				fill(j+1, 0)
			}
			{
				// startColumn overwrote this column's counts; rebuild them
				// so the caller's undo stays correct.
				startColumn(j)
				for r := 0; r < need; r++ {
					for jp := 0; jp < j; jp++ {
						x := int(M[r][jp])*3 + int(M[r][j])
						if cov[jp][x] == 0 {
							miss[jp]--
						}
						cov[jp][x]++
					}
				}
			}
			return
		}
		lo := uint8(0)
		if j == 0 {
			lo = 1 // new rows have column 0 in {1, 2}
		}
		if i > 0 && slices.Equal(M[i][:j], M[i-1][:j]) && M[i-1][j] > lo {
			lo = M[i-1][j]
		}
		left := need - i - 1
		for x := lo; x < 3; x++ {
			M[i][j] = x
			ok := true
			for jp := 0; jp < j; jp++ {
				p := int(M[i][jp])*3 + int(x)
				if cov[jp][p] == 0 {
					miss[jp]--
				}
				cov[jp][p]++
				if miss[jp] > left {
					ok = false
				}
			}
			if ok {
				fill(j, i+1)
			}
			for jp := 0; jp < j; jp++ {
				p := int(M[i][jp])*3 + int(x)
				cov[jp][p]--
				if cov[jp][p] == 0 {
					miss[jp]++
				}
			}
		}
	}
	fill(0, 0)
	return out
}

func rowBytes(s []Row) []byte {
	b := make([]byte, 0, 2*len(s))
	for _, r := range s {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

// interKey is the sorted multiset of rows in s with column c equal to x.
func interKey(s Slice, c int, x uint8) string {
	var rows []Row
	for _, r := range s {
		if r.At(c) == x {
			rows = append(rows, r)
		}
	}
	return string(rowBytes(rows)) // s is sorted, so rows is too
}

// Table holds every candidate slice for each (column, symbol).
type Table struct {
	A       []Row
	K, Size int
	Cands   [MaxK][3][]Slice
}

// Build enumerates candidates for columns 1..k-1 in parallel.
func Build(A []Row, k, size int) *Table {
	t := &Table{A: A, K: k, Size: size}
	var wg sync.WaitGroup
	for Y := 1; Y < k; Y++ {
		for v := uint8(0); v < 3; v++ {
			wg.Add(1)
			go func(Y int, v uint8) {
				defer wg.Done()
				t.Cands[Y][v] = Completions(A, k, size, Y, v)
			}(Y, v)
		}
	}
	wg.Wait()
	return t
}

// Compat counts compatible (B, C) pairs for B with Y = v and C with Z = w.
func (t *Table) Compat(Y, Z int, v, w uint8) int {
	keys := map[string]int{}
	for _, b := range t.Cands[Y][v] {
		keys[interKey(b, Z, w)]++
	}
	n := 0
	for _, c := range t.Cands[Z][w] {
		n += keys[interKey(c, Y, v)]
	}
	return n
}

// Matrix is the 3x3 table of compatible-pair counts for columns Y, Z.
func (t *Table) Matrix(Y, Z int) [3][3]int {
	var m [3][3]int
	for v := uint8(0); v < 3; v++ {
		for w := uint8(0); w < 3; w++ {
			m[v][w] = t.Compat(Y, Z, v, w)
		}
	}
	return m
}
