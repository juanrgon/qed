// Package girth5 searches for graphs with no triangle and no 4-cycle
// (girth at least 5) and many edges.
//
// The rerouting search: take a known graph G, pick a set R of k vertices,
// delete every edge that touches R, and ask a SAT solver whether edges
// touching R can be added back so the graph keeps girth at least 5 and
// reaches a target edge count. SAT gives a new graph (checked by
// cmd/verify-girth5). UNSAT gives an LRAT proof (checked by
// cmd/check-lrat), and the resulting claim is scoped: it only says this
// particular rerouting of this particular graph cannot reach the target.
package girth5

import (
	"fmt"
	"sort"

	"github.com/juanrgon/qed/internal/sat"
)

// Graph is an adjacency matrix.
type Graph [][]bool

// New returns an empty graph on n vertices.
func New(n int) Graph {
	g := make(Graph, n)
	for i := range g {
		g[i] = make([]bool, n)
	}
	return g
}

// Edges counts edges.
func (g Graph) Edges() int {
	e := 0
	for i := range g {
		for j := i + 1; j < len(g); j++ {
			if g[i][j] {
				e++
			}
		}
	}
	return e
}

// Clone copies g.
func (g Graph) Clone() Graph {
	h := New(len(g))
	for i := range g {
		copy(h[i], g[i])
	}
	return h
}

// ParseGraph6 decodes a graph6 string.
func ParseGraph6(s string) (Graph, error) {
	b := []byte(s)
	var n, pos int
	switch {
	case len(b) >= 1 && b[0] != 126:
		n, pos = int(b[0])-63, 1
	case len(b) >= 4 && b[1] != 126:
		n = (int(b[1])-63)<<12 | (int(b[2])-63)<<6 | (int(b[3]) - 63)
		pos = 4
	default:
		return nil, fmt.Errorf("unsupported graph6 header")
	}
	if len(b)-pos != (n*(n-1)/2+5)/6 {
		return nil, fmt.Errorf("graph6 length does not match n=%d", n)
	}
	g := New(n)
	k := 0
	for j := 1; j < n; j++ {
		for i := 0; i < j; i++ {
			c := int(b[pos+k/6]) - 63
			if c < 0 || c > 63 {
				return nil, fmt.Errorf("bad graph6 byte")
			}
			if c>>(5-k%6)&1 == 1 {
				g[i][j], g[j][i] = true, true
			}
			k++
		}
	}
	return g, nil
}

// Graph6 encodes g.
func (g Graph) Graph6() string {
	n := len(g)
	var out []byte
	if n < 63 {
		out = append(out, byte(n+63))
	} else {
		out = append(out, 126, byte(n>>12&63+63), byte(n>>6&63+63), byte(n&63+63))
	}
	c, k := 0, 0
	for j := 1; j < n; j++ {
		for i := 0; i < j; i++ {
			c <<= 1
			if g[i][j] {
				c |= 1
			}
			k++
			if k%6 == 0 {
				out = append(out, byte(c+63))
				c = 0
			}
		}
	}
	if k%6 != 0 {
		c <<= 6 - k%6
		out = append(out, byte(c+63))
	}
	return string(out)
}

// Reroute is one rerouting problem.
type Reroute struct {
	H      Graph    // G with every edge touching R removed
	R      []int    // the k rerouted vertices
	Vars   [][2]int // Vars[i] is the edge for SAT variable i+1
	Target int      // total edges wanted
	CNF    *sat.CNF
}

// NewReroute builds the SAT problem for rerouting R in g to reach target
// edges.
//
// Every edge not touching R is fixed (present or absent as in g). Every
// edge touching R is a variable. A triangle or 4-cycle whose fixed edges
// are all present must not have all its variable edges chosen; those are
// exactly the cycles that could appear. The solver must also choose at
// least target - |E(H)| variable edges.
func NewReroute(g Graph, R []int, target int) *Reroute {
	p := rerouteBase(g, R)
	p.Target = target
	var xs []int
	for i := range p.Vars {
		xs = append(xs, i+1)
	}
	p.CNF.AtLeast(xs, target-p.H.Edges())
	return p
}

// NewRerouteDegrees is NewReroute with a degree target per freed vertex
// instead of one global edge count: vertex r must end with at least
// minDeg[r] edges. Small per-vertex counters are much easier for the
// solver than one counter over every variable, at the price of a narrower
// neighborhood (edge counts cannot shift between freed vertices).
func NewRerouteDegrees(g Graph, R []int, minDeg map[int]int) *Reroute {
	p := rerouteBase(g, R)
	for _, r := range R {
		var xs []int
		for i, e := range p.Vars {
			if e[0] == r || e[1] == r {
				xs = append(xs, i+1)
			}
		}
		p.CNF.AtLeast(xs, minDeg[r])
	}
	return p
}

// rerouteBase builds the variables and the cycle clauses, with no count.
func rerouteBase(g Graph, R []int) *Reroute {
	n := len(g)
	inR := make([]bool, n)
	for _, r := range R {
		inR[r] = true
	}
	h := g.Clone()
	for _, r := range R {
		for j := 0; j < n; j++ {
			h[r][j], h[j][r] = false, false
		}
	}
	f := &sat.CNF{}
	varOf := make([][]int, n)
	for i := range varOf {
		varOf[i] = make([]int, n)
	}
	var vars [][2]int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if inR[i] || inR[j] {
				v := f.NewVar()
				varOf[i][j], varOf[j][i] = v, v
				vars = append(vars, [2]int{i, j})
			}
		}
	}
	// possible(i, j): the edge can be present (fixed present or variable).
	possible := func(i, j int) bool { return varOf[i][j] != 0 || h[i][j] }
	nbrs := make([][]int, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			if i != j && possible(i, j) {
				nbrs[i] = append(nbrs[i], j)
			}
		}
	}
	seen := map[string]bool{}
	addCycle := func(cyc []int) {
		var lits []int
		var key []int
		for i := range cyc {
			a, b := cyc[i], cyc[(i+1)%len(cyc)]
			if v := varOf[a][b]; v != 0 {
				lits = append(lits, -v)
			}
			if a > b {
				a, b = b, a
			}
			key = append(key, a*n+b)
		}
		if len(lits) == 0 {
			return // a cycle of fixed edges: g itself would be bad
		}
		sort.Ints(key)
		k := fmt.Sprint(key)
		if seen[k] {
			return
		}
		seen[k] = true
		f.Add(lits...)
	}
	// Every cycle with a variable edge passes through some r in R.
	for _, r := range R {
		for _, b := range nbrs[r] {
			for _, c := range nbrs[b] {
				if c == r {
					continue
				}
				if possible(c, r) {
					addCycle([]int{r, b, c})
				}
				for _, d := range nbrs[c] {
					if d == r || d == b {
						continue
					}
					if possible(d, r) {
						addCycle([]int{r, b, c, d})
					}
				}
			}
		}
	}
	return &Reroute{H: h, R: R, Vars: vars, CNF: f}
}

// Decode builds the graph from a SAT model.
func (p *Reroute) Decode(model []bool) Graph {
	g := p.H.Clone()
	for i, e := range p.Vars {
		if model[i+1] {
			g[e[0]][e[1]], g[e[1]][e[0]] = true, true
		}
	}
	return g
}
