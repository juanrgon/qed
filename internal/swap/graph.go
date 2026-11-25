package swap

import "math/bits"

type colorStatus int

const (
	found  colorStatus = iota // a coloring with k colors exists
	proven                    // exhaustive search: no k-coloring
	capped                    // node cap hit: unknown
)

// solver holds the conflict graph on one U, reused across subsets.
type solver struct {
	n     int
	words int
	adj   []uint64 // n*words
	deg   []int
	nodes int64
	// coloring state
	col []int
	sat   [][]int8 // sat[v][c] = neighbors of v with color c
}

func newSolver() *solver { return &solver{} }

func (s *solver) load(U []int, conflicts func(i, j int) bool) {
	s.n = len(U)
	s.words = (s.n + 63) / 64
	if cap(s.adj) < s.n*s.words {
		s.adj = make([]uint64, s.n*s.words)
	}
	s.adj = s.adj[:s.n*s.words]
	for i := range s.adj {
		s.adj[i] = 0
	}
	if cap(s.deg) < s.n {
		s.deg = make([]int, s.n)
	}
	s.deg = s.deg[:s.n]
	for i := range s.deg {
		s.deg[i] = 0
	}
	for i := 0; i < s.n; i++ {
		for j := i + 1; j < s.n; j++ {
			if conflicts(U[i], U[j]) {
				s.adj[i*s.words+j/64] |= 1 << uint(j%64)
				s.adj[j*s.words+i/64] |= 1 << uint(i%64)
				s.deg[i]++
				s.deg[j]++
			}
		}
	}
}

func (s *solver) row(i int) []uint64 { return s.adj[i*s.words : (i+1)*s.words] }

// hasClique reports whether the graph has a clique of size m. It returns
// false if the node cap is hit; the caller then falls back to coloring.
func (s *solver) hasClique(m int, nodeCap int64) bool {
	if m <= 1 {
		return s.n >= m
	}
	s.nodes = 0
	P := make([]uint64, s.words)
	for i := 0; i < s.n; i++ {
		P[i/64] |= 1 << uint(i%64)
	}
	return s.expand(0, m, P, nodeCap)
}

// expand looks for need-size more clique vertices inside P, where every
// vertex of P is adjacent to the size vertices already chosen.
func (s *solver) expand(size, need int, P []uint64, nodeCap int64) bool {
	s.nodes++
	if s.nodes > nodeCap {
		return false
	}
	cnt := 0
	for _, w := range P {
		cnt += bits.OnesCount64(w)
	}
	if size+cnt < need {
		return false
	}
	// Greedy coloring bound: a clique uses one vertex per color class.
	if size+s.greedyColors(P, need-size) < need {
		return false
	}
	next := make([]uint64, s.words)
	for wi := range P {
		for P[wi] != 0 {
			b := bits.TrailingZeros64(P[wi])
			v := wi*64 + b
			P[wi] &^= 1 << uint(b)
			if size+1 >= need {
				return true
			}
			nv := s.row(v)
			any := false
			for k := range next {
				next[k] = P[k] & nv[k]
				if next[k] != 0 {
					any = true
				}
			}
			if any && s.expand(size+1, need, next, nodeCap) {
				return true
			}
			if s.nodes > nodeCap {
				return false
			}
		}
	}
	return false
}

// greedyColors colors P greedily and returns the color count, stopping
// early once it reaches limit (we only need to know if it is >= limit).
func (s *solver) greedyColors(P []uint64, limit int) int {
	left := make([]uint64, s.words)
	copy(left, P)
	colors := 0
	cls := make([]uint64, s.words)
	for {
		empty := true
		for _, w := range left {
			if w != 0 {
				empty = false
				break
			}
		}
		if empty || colors >= limit {
			return colors
		}
		colors++
		copy(cls, left)
		for wi := range cls {
			for cls[wi] != 0 {
				b := bits.TrailingZeros64(cls[wi])
				v := wi*64 + b
				cls[wi] &^= 1 << uint(b)
				left[wi] &^= 1 << uint(b)
				nv := s.row(v)
				for k := range cls {
					cls[k] &^= nv[k]
				}
			}
		}
	}
}

// color tries to color the graph with k colors using DSATUR backtracking.
func (s *solver) color(k int, nodeCap int64) ([]int, colorStatus) {
	s.nodes = 0
	if cap(s.col) < s.n {
		s.col = make([]int, s.n)
	}
	s.col = s.col[:s.n]
	for i := range s.col {
		s.col[i] = -1
	}
	if len(s.sat) < s.n || (len(s.sat) > 0 && len(s.sat[0]) < k) {
		s.sat = make([][]int8, s.n)
		for i := range s.sat {
			s.sat[i] = make([]int8, max(k, 1))
		}
	}
	for i := 0; i < s.n; i++ {
		for c := 0; c < k; c++ {
			s.sat[i][c] = 0
		}
	}
	ok, hitCap := s.dsatur(0, k, 0, nodeCap)
	switch {
	case ok:
		return append([]int(nil), s.col...), found
	case hitCap:
		return nil, capped
	default:
		return nil, proven
	}
}

func (s *solver) dsatur(done, k, used int, nodeCap int64) (bool, bool) {
	if done == s.n {
		return true, false
	}
	s.nodes++
	if s.nodes > nodeCap {
		return false, true
	}
	// Pick the uncolored vertex with the most distinct neighbor colors.
	best, bestSat, bestDeg := -1, -1, -1
	for v := 0; v < s.n; v++ {
		if s.col[v] >= 0 {
			continue
		}
		sat := 0
		for c := 0; c < k; c++ {
			if s.sat[v][c] > 0 {
				sat++
			}
		}
		if sat > bestSat || (sat == bestSat && s.deg[v] > bestDeg) {
			best, bestSat, bestDeg = v, sat, s.deg[v]
		}
	}
	if bestSat >= k {
		return false, false
	}
	limit := min(k, used+1) // new colors are interchangeable
	nv := s.row(best)
	for c := 0; c < limit; c++ {
		if s.sat[best][c] > 0 {
			continue
		}
		s.col[best] = c
		s.bump(nv, c, 1)
		nu := used
		if c == used {
			nu++
		}
		ok, hitCap := s.dsatur(done+1, k, nu, nodeCap)
		if ok || hitCap {
			if !ok {
				s.bump(nv, c, -1)
				s.col[best] = -1
			}
			return ok, hitCap
		}
		s.bump(nv, c, -1)
		s.col[best] = -1
	}
	return false, false
}

func (s *solver) bump(nv []uint64, c int, d int8) {
	for wi, w := range nv {
		for w != 0 {
			b := bits.TrailingZeros64(w)
			w &^= 1 << uint(b)
			s.sat[wi*64+b][c] += d
		}
	}
}
