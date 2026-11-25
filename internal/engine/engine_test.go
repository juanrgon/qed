package engine

import (
	"math/rand/v2"
	"testing"

	"github.com/juanrgon/mathbench/internal/ca"
)

// The incremental counts must agree with the naive verifier after many
// random edits.
func TestIncrementalMatchesVerifier(t *testing.T) {
	ix := ca.NewIndex(3, 8, 3)
	a := ca.New(30, 8)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range a.Cells {
		a.Cells[i] = uint8(rng.IntN(3))
	}
	st := NewState(ix, a)
	want := st.Missing()
	for i := 0; i < 20000; i++ {
		want += st.Set(rng.IntN(30), rng.IntN(8), uint8(rng.IntN(3)))
		if i%1000 == 0 {
			if got := ca.Verify(a, 3, 3).Missing; got != st.Missing() || got != want {
				t.Fatalf("step %d: verifier %d, state %d, running %d", i, got, st.Missing(), want)
			}
		}
	}
}

func TestPublishedArrayIsShiftInvariant(t *testing.T) {
	a, err := ca.ReadFile("../ca/testdata/CA_42_3_8_3.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]bool{}
	for r := 0; r < a.N; r++ {
		rows[string(a.Row(r))] = true
	}
	for r := 0; r < a.N; r++ {
		sh := make([]byte, a.K)
		for c, x := range a.Row(r) {
			sh[c] = (x + 1) % 3
		}
		if !rows[string(sh)] {
			t.Fatalf("row %d shifted is not in the array", r)
		}
	}
}

func TestAnnealSolvesEasySizes(t *testing.T) {
	ix := ca.NewIndex(3, 8, 3)
	models := []*Model{Free(50, 8, 3), SplitColumn0(8, 3, []int{16, 17, 17}), ShiftOrbits(16, 2, 8, 3)}
	for _, m := range models {
		p := DefaultParams()
		p.Moves = 5_000_000
		p.Seed = 7
		out := Anneal(m, ix, p)
		if !out.Solved {
			t.Fatalf("%s: not solved, best %d", m.Name, out.Best)
		}
		if !ca.Verify(out.Array, 3, 3).OK() {
			t.Fatalf("%s: engine claimed a solve the verifier rejects", m.Name)
		}
	}
}

func TestSameSeedSameResult(t *testing.T) {
	ix := ca.NewIndex(3, 8, 3)
	p := DefaultParams()
	p.Moves, p.Seed = 200_000, 42
	a := Anneal(Free(44, 8, 3), ix, p)
	b := Anneal(Free(44, 8, 3), ix, p)
	if a.Best != b.Best || a.Moves != b.Moves {
		t.Fatalf("runs differ: %+v vs %+v", a, b)
	}
}
