// Package swap asks: can some m rows of a covering array be replaced by
// m-1 new rows and keep full coverage? For each m-subset S of rows it
// collects U, the requirements covered only by rows in S. New rows must
// cover U. A set of requirements fits in one row exactly when no two of
// them put different values in the same column, so the fewest new rows
// needed is the chromatic number of the "conflict graph" on U.
//
// Each subset ends in one of three states:
//   - Impossible: proven, by an m-clique of pairwise conflicting
//     requirements or by an exhaustive (m-1)-coloring search.
//   - Witness: an (m-1)-coloring exists. The rebuilt N-1 row array is
//     checked by the independent verifier before it counts.
//   - Undetermined: the search hit its node cap. Never treated as proof.
package swap

import (
	"math/bits"
	"sync"
	"sync/atomic"

	"github.com/juanrgon/qed/internal/ca"
)

// Summary aggregates the outcome over all m-subsets.
type Summary struct {
	M            int
	Subsets      int64
	ByClique     int64 // impossible, certified by an m-clique
	ByColoring   int64 // impossible, certified by exhaustive coloring
	Undetermined int64
	Witnesses    []*ca.Array // verified smaller arrays (should be empty)
	BadWitnesses int64       // colorings the verifier rejected: a bug if > 0
	MaxU         int         // largest U seen
}

// Impossible is the number of subsets proven not replaceable.
func (s Summary) Impossible() int64 { return s.ByClique + s.ByColoring }

type req struct {
	id   int
	mask uint64
	cols []int
	vals []uint8
}

// Cert is the proof that one m-subset cannot be replaced: m requirements
// (each a list of columns and values) that only rows in the subset cover
// and that pairwise put different values in some shared column. New rows
// would need one row per requirement, so m-1 rows cannot cover them.
type Cert struct {
	Rows uint64 // the subset, as a bit mask of row indices
	Cols [][]int
	Vals [][]uint8
}

// Check runs the m -> m-1 test over every m-subset of rows of a.
// nodeCap bounds each clique or coloring search.
func Check(a *ca.Array, t, v, m, workers int, nodeCap int64) Summary {
	return CheckCerts(a, t, v, m, workers, nodeCap, nil)
}

// CheckCerts is Check, and also calls emit with the clique certificate of
// every subset proven by a clique. emit may be called from several
// goroutines at once.
func CheckCerts(a *ca.Array, t, v, m, workers int, nodeCap int64, emit func(Cert)) Summary {
	if a.N > 64 {
		panic("swap: at most 64 rows supported")
	}
	ix := ca.NewIndex(t, a.K, v)

	// Cover mask for every requirement.
	masks := make([]uint64, ix.NumReq())
	for r := 0; r < a.N; r++ {
		row := a.Row(r)
		for ci := range ix.Combos {
			masks[ci*ix.Pow+ix.Code(row, ci)] |= 1 << uint(r)
		}
	}
	// Only requirements covered by at most m rows can land in U.
	var L []req
	for id, mk := range masks {
		if mk == 0 {
			panic("swap: input is not a covering array")
		}
		if bits.OnesCount64(mk) <= m {
			cols, vals := ix.Decode(id)
			L = append(L, req{id: id, mask: mk, cols: cols, vals: vals})
		}
	}
	// Group by lowest covering row: a requirement is in U(S) only if its
	// lowest row is in S, so each subset scans m small lists.
	byLow := make([][]int, a.N)
	for i, q := range L {
		byLow[bits.TrailingZeros64(q.mask)] = append(byLow[bits.TrailingZeros64(q.mask)], i)
	}
	// Full conflict matrix over L.
	words := (len(L) + 63) / 64
	conflict := make([]uint64, len(L)*words)
	colVal := make([][]int8, len(L))
	for i, q := range L {
		cv := make([]int8, a.K)
		for c := range cv {
			cv[c] = -1
		}
		for j, c := range q.cols {
			cv[c] = int8(q.vals[j])
		}
		colVal[i] = cv
	}
	for i := range L {
		for j := i + 1; j < len(L); j++ {
			for c := 0; c < a.K; c++ {
				x, y := colVal[i][c], colVal[j][c]
				if x >= 0 && y >= 0 && x != y {
					conflict[i*words+j/64] |= 1 << uint(j%64)
					conflict[j*words+i/64] |= 1 << uint(i%64)
					break
				}
			}
		}
	}
	conflicts := func(i, j int) bool { return conflict[i*words+j/64]>>uint(j%64)&1 == 1 }

	// Work items: the first two rows of each subset.
	type prefix struct{ a, b int }
	work := make(chan prefix, 256)
	go func() {
		for i := 0; i <= a.N-m; i++ {
			if m == 1 {
				work <- prefix{i, -1}
				continue
			}
			for j := i + 1; j <= a.N-m+1; j++ {
				work <- prefix{i, j}
			}
		}
		close(work)
	}()

	var sum Summary
	sum.M = m
	var mu sync.Mutex
	var subsets, byClique, byColoring, undet, bad atomic.Int64
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sol := newSolver()
			rows := make([]int, m)
			U := make([]int, 0, 512)
			maxU := 0
			var visit func(depth, start int, S uint64)
			visit = func(depth, start int, S uint64) {
				if depth == m {
					subsets.Add(1)
					U = U[:0]
					for _, r := range rows {
						for _, i := range byLow[r] {
							if L[i].mask&^S == 0 {
								U = append(U, i)
							}
						}
					}
					if len(U) > maxU {
						maxU = len(U)
					}
					sol.load(U, conflicts)
					if sol.hasClique(m, nodeCap) {
						byClique.Add(1)
						if emit != nil {
							c := Cert{Rows: S}
							for _, vi := range sol.clique {
								q := L[U[vi]]
								c.Cols = append(c.Cols, q.cols)
								c.Vals = append(c.Vals, q.vals)
							}
							emit(c)
						}
						return
					}
					colors, status := sol.color(m-1, nodeCap)
					switch status {
					case proven:
						byColoring.Add(1)
					case capped:
						undet.Add(1)
					case found:
						drop := map[int]bool{}
						for _, r := range rows {
							drop[r] = true
						}
						b := a.Without(drop)
						for k := 0; k < m-1; k++ {
							row := make([]uint8, a.K)
							for vi, col := range colors {
								if col != k {
									continue
								}
								for c, x := range colVal[U[vi]] {
									if x >= 0 {
										row[c] = uint8(x)
									}
								}
							}
							b.AppendRow(row)
						}
						if ca.Verify(b, t, v).OK() {
							mu.Lock()
							sum.Witnesses = append(sum.Witnesses, b)
							mu.Unlock()
						} else {
							bad.Add(1)
						}
					}
					return
				}
				for r := start; r <= a.N-(m-depth); r++ {
					rows[depth] = r
					visit(depth+1, r+1, S|1<<uint(r))
				}
			}
			for p := range work {
				rows[0] = p.a
				S := uint64(1) << uint(p.a)
				if m == 1 {
					visit(1, p.a+1, S)
					continue
				}
				rows[1] = p.b
				visit(2, p.b+1, S|1<<uint(p.b))
			}
			mu.Lock()
			if maxU > sum.MaxU {
				sum.MaxU = maxU
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	sum.Subsets = subsets.Load()
	sum.ByClique = byClique.Load()
	sum.ByColoring = byColoring.Load()
	sum.Undetermined = undet.Load()
	sum.BadWitnesses = bad.Load()
	return sum
}
