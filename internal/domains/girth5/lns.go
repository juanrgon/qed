package girth5

import (
	"math/rand/v2"

	"github.com/juanrgon/qed/internal/sat"
)

// Solver answers a CNF query. Search loops use a fast solver with no
// proof: an UNSAT answer there only steers the search.
type Solver func(f *sat.CNF) (sat.Result, error)

// LNSParams configures the SAT-based large neighborhood search.
type LNSParams struct {
	Iters int     // neighborhoods to try
	K     int     // vertices freed per neighborhood
	Seed  uint64  // the run is a pure function of the seed
	Ball  float64 // chance of picking a local ball instead of random vertices
	Stop  int     // stop early once the graph has this many edges (0: never)
	// Degrees uses per-vertex degree targets instead of one global edge
	// count (see NewRerouteDegrees).
	Degrees bool
}

// LNSStats summarizes a run.
type LNSStats struct {
	Iters, Improved, Plateau, Stuck, Unknown int
}

// CanAdd reports whether edge a-b can be added to g keeping girth at
// least 5: a and b must not be adjacent, share no neighbor (a triangle)
// and have no path of length 3 (a 4-cycle).
func CanAdd(g Graph, a, b int) bool {
	if a == b || g[a][b] {
		return false
	}
	n := len(g)
	for x := 0; x < n; x++ {
		if !g[a][x] {
			continue
		}
		if g[b][x] {
			return false
		}
		for y := 0; y < n; y++ {
			if g[x][y] && g[b][y] {
				return false
			}
		}
	}
	return true
}

// Greedy adds random edges to g while it keeps girth at least 5.
func Greedy(g Graph, rng *rand.Rand) {
	n := len(g)
	var pairs [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			pairs = append(pairs, [2]int{i, j})
		}
	}
	rng.Shuffle(len(pairs), func(i, j int) { pairs[i], pairs[j] = pairs[j], pairs[i] })
	for _, p := range pairs {
		if CanAdd(g, p[0], p[1]) {
			g[p[0]][p[1]], g[p[1]][p[0]] = true, true
		}
	}
}

// pick chooses the k vertices to free: either uniformly at random, or a
// random vertex plus random vertices within distance 2 of it, so the
// freed edges interact.
func pick(g Graph, k int, ball float64, rng *rand.Rand) []int {
	n := len(g)
	in := make([]bool, n)
	var R []int
	add := func(v int) {
		if !in[v] && len(R) < k {
			in[v] = true
			R = append(R, v)
		}
	}
	if rng.Float64() < ball {
		v := rng.IntN(n)
		add(v)
		var near []int
		for x := 0; x < n; x++ {
			if g[v][x] {
				near = append(near, x)
				for y := 0; y < n; y++ {
					if g[x][y] && y != v {
						near = append(near, y)
					}
				}
			}
		}
		rng.Shuffle(len(near), func(i, j int) { near[i], near[j] = near[j], near[i] })
		for _, x := range near {
			add(x)
		}
	}
	for len(R) < k {
		add(rng.IntN(n))
	}
	return R
}

// LNS improves g by repeatedly freeing K vertices and asking the solver
// for one more edge. If there is none, it asks for a different graph with
// the same number of edges (a plateau move), so the search keeps moving.
// onImprove is called with each new best graph.
func LNS(g Graph, p LNSParams, solve Solver, onImprove func(Graph)) (Graph, LNSStats, error) {
	rng := rand.New(rand.NewPCG(p.Seed, 0x9e3779b97f4a7c15))
	g = g.Clone()
	e := g.Edges()
	var st LNSStats
	for st.Iters = 0; st.Iters < p.Iters; st.Iters++ {
		if p.Stop > 0 && e >= p.Stop {
			break
		}
		R := pick(g, p.K, p.Ball, rng)
		build := func(extra int) *Reroute {
			if !p.Degrees {
				return NewReroute(g, R, e+extra)
			}
			min := map[int]int{}
			for _, r := range R {
				for x := range g {
					if g[r][x] {
						min[r]++
					}
				}
			}
			min[R[rng.IntN(len(R))]] += extra
			return NewRerouteDegrees(g, R, min)
		}
		up := build(1)
		res, err := solve(up.CNF)
		if err != nil {
			return g, st, err
		}
		if res.Unknown {
			st.Unknown++
		}
		if res.Sat {
			// Degree targets do not by themselves guarantee more edges
			// (edges inside R count twice), so check the count.
			if h := up.Decode(res.Model); h.Edges() > e {
				g, e = h, h.Edges()
				st.Improved++
				if onImprove != nil {
					onImprove(g)
				}
				continue
			}
		}
		// Plateau: same edge count, but not the current configuration.
		flat := build(0)
		var block []int
		for i, ed := range flat.Vars {
			if g[ed[0]][ed[1]] {
				block = append(block, -(i + 1))
			} else {
				block = append(block, i+1)
			}
		}
		flat.CNF.Add(block...)
		res, err = solve(flat.CNF)
		if err != nil {
			return g, st, err
		}
		if res.Sat {
			if h := flat.Decode(res.Model); h.Edges() >= e {
				g, e = h, h.Edges()
				st.Plateau++
				continue
			}
		}
		st.Stuck++
	}
	return g, st, nil
}
