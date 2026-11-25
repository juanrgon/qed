package swap

import (
	"runtime"
	"testing"

	"github.com/juanrgon/mathbench/internal/ca"
)

func witness(t *testing.T) *ca.Array {
	t.Helper()
	a, err := ca.ReadFile("../ca/testdata/CA_42_3_8_3.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// Reproduces the panel's claims: no 3 rows -> 2, no 4 -> 3 (and 5 -> 4 in
// the long test), with the exact subset counts they quoted.
func TestPanelClaims(t *testing.T) {
	a := witness(t)
	cases := []struct {
		m    int
		want int64
	}{{2, 861}, {3, 11480}, {4, 111930}}
	if !testing.Short() {
		cases = append(cases, struct {
			m    int
			want int64
		}{5, 850668})
	}
	for _, c := range cases {
		s := Check(a, 3, 3, c.m, runtime.NumCPU(), 1<<20)
		if s.Subsets != c.want {
			t.Fatalf("m=%d: %d subsets, want %d", c.m, s.Subsets, c.want)
		}
		if s.Impossible() != c.want || len(s.Witnesses) != 0 || s.BadWitnesses != 0 || s.Undetermined != 0 {
			t.Fatalf("m=%d: %+v", c.m, s)
		}
	}
}

// Sanity check the other direction: pad a covering array with a duplicate
// row, and the checker must find the 2 -> 1 swap and verify it.
func TestFindsRealSwap(t *testing.T) {
	a := witness(t)
	b := a.Clone()
	b.AppendRow(append([]uint8(nil), a.Row(5)...))
	s := Check(b, 3, 3, 2, 2, 1<<20)
	if len(s.Witnesses) == 0 {
		t.Fatalf("expected a witness, got %+v", s)
	}
	for _, w := range s.Witnesses {
		if w.N != 42 || !ca.Verify(w, 3, 3).OK() {
			t.Fatalf("bad witness")
		}
	}
	if s.BadWitnesses != 0 {
		t.Fatalf("verifier rejected %d colorings", s.BadWitnesses)
	}
}
