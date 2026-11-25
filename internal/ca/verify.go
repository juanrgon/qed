package ca

// This file is the judge. It is deliberately naive and shares no code with
// the incremental engine, so a bug in the fast path cannot fake a solve.

// Gap is one uncovered requirement: a set of columns and the values they
// never show together.
type Gap struct {
	Cols []int
	Vals []uint8
}

// Report is the result of a full verification.
type Report struct {
	Missing  int   // total uncovered requirements
	Examples []Gap // up to 20 of them
}

// OK reports whether every requirement is covered.
func (r Report) OK() bool { return r.Missing == 0 }

// Verify checks that every t columns of a show all v^t value tuples.
func Verify(a *Array, t, v int) Report {
	var rep Report
	pow := 1
	for i := 0; i < t; i++ {
		pow *= v
	}
	seen := make([]bool, pow)
	for _, cols := range Combinations(a.K, t) {
		for i := range seen {
			seen[i] = false
		}
		for r := 0; r < a.N; r++ {
			code := 0
			for _, c := range cols {
				code = code*v + int(a.At(r, c))
			}
			seen[code] = true
		}
		for code, ok := range seen {
			if ok {
				continue
			}
			rep.Missing++
			if len(rep.Examples) < 20 {
				vals := make([]uint8, t)
				x := code
				for j := t - 1; j >= 0; j-- {
					vals[j] = uint8(x % v)
					x /= v
				}
				rep.Examples = append(rep.Examples, Gap{Cols: append([]int(nil), cols...), Vals: vals})
			}
		}
	}
	return rep
}

// Combinations lists all t-subsets of {0..k-1} in lexicographic order.
func Combinations(k, t int) [][]int {
	var out [][]int
	cur := make([]int, t)
	var rec func(start, depth int)
	rec = func(start, depth int) {
		if depth == t {
			out = append(out, append([]int(nil), cur...))
			return
		}
		for i := start; i <= k-(t-depth); i++ {
			cur[depth] = i
			rec(i+1, depth+1)
		}
	}
	rec(0, 0)
	return out
}
