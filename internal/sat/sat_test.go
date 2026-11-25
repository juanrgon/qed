package sat

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// girth5 encodes "a graph on n vertices with at least e edges and no
// triangle or 4-cycle". edge[i][j] is the variable for edge {i, j}.
func girth5(n, e int) (*CNF, [][]int) {
	f := &CNF{}
	edge := make([][]int, n)
	for i := range edge {
		edge[i] = make([]int, n)
	}
	var all []int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			v := f.NewVar()
			edge[i][j], edge[j][i] = v, v
			all = append(all, v)
		}
	}
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			for c := b + 1; c < n; c++ {
				f.Add(-edge[a][b], -edge[b][c], -edge[a][c])
				for d := c + 1; d < n; d++ {
					// the three 4-cycles on {a, b, c, d}
					f.Add(-edge[a][b], -edge[b][c], -edge[c][d], -edge[d][a])
					f.Add(-edge[a][b], -edge[b][d], -edge[d][c], -edge[c][a])
					f.Add(-edge[a][c], -edge[c][b], -edge[b][d], -edge[d][a])
				}
			}
		}
	}
	f.AtLeast(all, e)
	return f, edge
}

// plainGirthCheck counts edges and looks for a triangle or a 4-cycle the
// slow way, from the adjacency matrix alone.
func plainGirthCheck(adj [][]bool) (edges int, ok bool) {
	n := len(adj)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if adj[i][j] {
				edges++
			}
		}
	}
	for a := 0; a < n; a++ {
		for b := 0; b < n; b++ {
			if a == b {
				continue
			}
			common := 0
			for c := 0; c < n; c++ {
				if c != a && c != b && adj[a][c] && adj[b][c] {
					common++
				}
			}
			// a common neighbor of adjacent a, b is a triangle; two common
			// neighbors of any a, b make a 4-cycle.
			if common >= 2 || (common == 1 && adj[a][b]) {
				return edges, false
			}
		}
	}
	return edges, true
}

func checkLRAT(t *testing.T, cnf, proof string) error {
	t.Helper()
	out, err := exec.Command("go", "run", "../../cmd/check-lrat", cnf, proof).CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	if !strings.HasPrefix(string(out), "VERIFIED UNSAT") {
		t.Fatalf("unexpected checker output: %s", out)
	}
	return nil
}

// The step 0 smoke test: on 10 vertices, 15 edges with no triangle or
// 4-cycle is possible (the Petersen graph), 16 is not.
func TestPetersen(t *testing.T) {
	dir := t.TempDir()
	f, edge := girth5(10, 15)
	res, err := Solve(f, filepath.Join(dir, "15.cnf"), filepath.Join(dir, "15.lrat"))
	if errors.Is(err, ErrNoSolver) {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !res.Sat {
		t.Fatal("15 edges should be satisfiable")
	}
	adj := make([][]bool, 10)
	for i := range adj {
		adj[i] = make([]bool, 10)
		for j := range adj[i] {
			adj[i][j] = i != j && res.Model[edge[i][j]]
		}
	}
	if e, ok := plainGirthCheck(adj); !ok || e < 15 {
		t.Fatalf("decoded graph: %d edges, girth ok %v", e, ok)
	}

	f, _ = girth5(10, 16)
	cnf, proof := filepath.Join(dir, "16.cnf"), filepath.Join(dir, "16.lrat")
	res, err = Solve(f, cnf, proof)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sat {
		t.Fatal("16 edges should be unsatisfiable")
	}
	if err := checkLRAT(t, cnf, proof); err != nil {
		t.Fatalf("real proof rejected: %v", err)
	}

	// Damage: drop a hint from the last proof step, and change the formula.
	raw, err := os.ReadFile(proof)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var steps []int
	for i, l := range lines {
		if f := strings.Fields(l); len(f) > 2 && f[1] != "d" {
			steps = append(steps, i)
		}
	}
	last := steps[len(steps)-1]
	f2 := strings.Fields(lines[last])
	// remove the final hint (just before the closing 0)
	cut := append(append([]string{}, f2[:len(f2)-2]...), "0")
	lines[last] = strings.Join(cut, " ")
	bad := filepath.Join(dir, "bad.lrat")
	os.WriteFile(bad, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	if err := checkLRAT(t, cnf, bad); err == nil {
		t.Error("proof with a missing hint was accepted")
	}

	// A proof for 16 edges must not check against the 15-edge formula.
	if err := checkLRAT(t, filepath.Join(dir, "15.cnf"), proof); err == nil {
		t.Error("proof accepted against a different, satisfiable formula")
	}
}

func TestAtMostCounts(t *testing.T) {
	// AtLeast(k) on 5 free variables: exactly the assignments with >= k
	// true must extend to a model. Check by brute force over the counter.
	for k := 0; k <= 5; k++ {
		for mask := 0; mask < 32; mask++ {
			f := &CNF{}
			xs := make([]int, 5)
			for i := range xs {
				xs[i] = f.NewVar()
			}
			f.AtLeast(xs, k)
			for i, x := range xs {
				if mask>>i&1 == 1 {
					f.Add(x)
				} else {
					f.Add(-x)
				}
			}
			dir := t.TempDir()
			res, err := Solve(f, filepath.Join(dir, "a.cnf"), filepath.Join(dir, "a.lrat"))
			if errors.Is(err, ErrNoSolver) {
				t.Skip(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			ones := 0
			for i := 0; i < 5; i++ {
				ones += mask >> i & 1
			}
			if res.Sat != (ones >= k) {
				t.Fatalf("k=%d mask=%05b: sat=%v", k, mask, res.Sat)
			}
		}
	}
}
