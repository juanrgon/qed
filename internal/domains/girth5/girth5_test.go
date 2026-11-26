package girth5

import (
	"errors"
	"math/rand/v2"
	"path/filepath"
	"testing"

	"github.com/juanrgon/qed/internal/sat"
)

// plainOK is a slow, separate girth check used only by tests.
func plainOK(g Graph) bool {
	n := len(g)
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			common := 0
			for c := 0; c < n; c++ {
				if g[a][c] && g[b][c] {
					common++
				}
			}
			if common >= 2 || (common == 1 && g[a][b]) {
				return false
			}
		}
	}
	return true
}

func petersen() Graph {
	g := New(10)
	add := func(a, b int) { g[a][b], g[b][a] = true, true }
	for i := 0; i < 5; i++ {
		add(i, (i+1)%5)     // outer cycle
		add(5+i, 5+(i+2)%5) // inner star
		add(i, 5+i)         // spokes
	}
	return g
}

func solve(t *testing.T, p *Reroute) (sat.Result, bool) {
	t.Helper()
	dir := t.TempDir()
	res, err := sat.Solve(p.CNF, filepath.Join(dir, "f.cnf"), filepath.Join(dir, "f.lrat"))
	if errors.Is(err, sat.ErrNoSolver) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	return res, true
}

func TestGraph6RoundTrip(t *testing.T) {
	g := petersen()
	h, err := ParseGraph6(g.Graph6())
	if err != nil {
		t.Fatal(err)
	}
	for i := range g {
		for j := range g {
			if g[i][j] != h[i][j] {
				t.Fatal("round trip changed the graph")
			}
		}
	}
	if g.Edges() != 15 || !plainOK(g) {
		t.Fatal("Petersen graph is wrong")
	}
}

func TestPetersenReroute(t *testing.T) {
	g := petersen()
	for _, R := range [][]int{{0}, {0, 7}, {1, 2, 8}} {
		res, _ := solve(t, NewReroute(g, R, 15))
		if !res.Sat {
			t.Fatalf("R=%v: 15 edges should be reachable (the original is a witness)", R)
		}
		p := NewReroute(g, R, 15)
		res, _ = solve(t, p)
		h := p.Decode(res.Model)
		if !plainOK(h) || h.Edges() < 15 {
			t.Fatalf("R=%v: decoded graph is bad", R)
		}
		if res, _ := solve(t, NewReroute(g, R, 16)); res.Sat {
			t.Fatalf("R=%v: 16 edges on 10 vertices is impossible", R)
		}
	}
}

// On small random girth-5 graphs, the SAT answer must match brute force
// over every way to re-add edges at one vertex.
func TestAgainstBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 9))
	for trial := 0; trial < 30; trial++ {
		n := 9
		g := New(n)
		for tries := 0; tries < 60; tries++ {
			a, b := rng.IntN(n), rng.IntN(n)
			if a == b || g[a][b] {
				continue
			}
			g[a][b], g[b][a] = true, true
			if !plainOK(g) {
				g[a][b], g[b][a] = false, false
			}
		}
		r := rng.IntN(n)
		best := 0
		others := []int{}
		for j := 0; j < n; j++ {
			if j != r {
				others = append(others, j)
			}
		}
		h := g.Clone()
		for j := 0; j < n; j++ {
			h[r][j], h[j][r] = false, false
		}
		for mask := 0; mask < 1<<len(others); mask++ {
			x := h.Clone()
			for i, j := range others {
				if mask>>i&1 == 1 {
					x[r][j], x[j][r] = true, true
				}
			}
			if plainOK(x) && x.Edges() > best {
				best = x.Edges()
			}
		}
		for target := best; target <= best+1; target++ {
			res, _ := solve(t, NewReroute(g, []int{r}, target))
			if res.Sat != (target <= best) {
				t.Fatalf("trial %d: target %d sat=%v, brute force max %d", trial, target, res.Sat, best)
			}
		}
	}
}
