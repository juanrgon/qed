package classify

import (
	"math/rand/v2"
	"testing"

	"github.com/juanrgon/qed/internal/ca"
)

func load(t *testing.T, path string) Array {
	t.Helper()
	a, err := ca.ReadFile(path, 3)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([][]uint8, a.N)
	for i := range rows {
		rows[i] = a.Row(i)
	}
	return FromRows(rows)
}

// counts runs Levels and returns the number of classes per column count.
func counts(n, k int) []int {
	var out []int
	for _, l := range Levels(n, k, 3) {
		out = append(out, len(l))
	}
	return out
}

// Known values used as controls:
//   - OA(9; 2, 4, 3) is unique (the affine plane of order 3).
//   - CAN(2, 6, 3) = 12, so no CA(11; 2, 6, 3) exists.
//   - There are 13 non-isomorphic CA(12; 2, 6, 3): Izquierdo-Marquez and
//     Torres-Jimenez, "New covering array numbers", arXiv 1711.10040.
//   - CA(12; 2, 7, 3) is unique: Colbourn, Kéri, Rivas Soriano,
//     Schlage-Puchta (2010) and Kokkala et al., arXiv 1901.03594.
func TestKnownCounts(t *testing.T) {
	if got := counts(9, 4); got[3] != 1 {
		t.Errorf("OA(9;2,4,3): %d classes, want 1", got[3])
	}
	c11 := counts(11, 6)
	if c11[5] != 0 {
		t.Errorf("CA(11;2,6,3): %d classes, want 0", c11[5])
	}
	if c11[4] == 0 {
		t.Errorf("CA(11;2,5,3) should exist (CAN(2,5,3) = 11)")
	}
	c12 := counts(12, 7)
	if c12[5] != 13 {
		t.Errorf("CA(12;2,6,3): %d classes, want 13", c12[5])
	}
	if c12[6] != 1 {
		t.Errorf("CA(12;2,7,3): %d classes, want 1", c12[6])
	}
	t.Logf("classes per column count: N=11 %v, N=12 %v", c11, c12)
}

// Every cell of Kokkala et al., arXiv 1901.03594, Table 1 for v = 3 and
// N = 10, 11, 12 (number of inequivalent CA(N; 2, k, 3), k = 2, 3, ...).
func TestKokkalaTable1(t *testing.T) {
	table := map[int][]int{
		10: {1, 3, 2, 0},
		11: {3, 20, 27, 3, 0},
		12: {7, 134, 987, 891, 13, 1, 0},
	}
	for n, want := range table {
		got := counts(n, len(want)+1)[1:] // drop k = 1
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("N=%d k=%d: %d classes, table says %d", n, i+2, got[i], want[i])
			}
		}
	}
}

// The light-slice proof pins column 0's slice to the table's copy of
// CA(12; 2, 7, 3). That is only safe if every CA(12; 2, 7, 3) is
// isomorphic to it, and if no CA(11; 2, 7, 3) exists.
func TestPinnedSliceIsTheOnlyClass(t *testing.T) {
	levels := Levels(12, 7, 3)
	if len(levels[6]) != 1 {
		t.Fatalf("CA(12;2,7,3) has %d classes, want 1", len(levels[6]))
	}
	rep := levels[6][0]
	if !IsCA(rep, 3) {
		t.Fatal("representative is not a covering array")
	}
	for _, f := range []string{"CA_12_2_7_3.txt", "CA_12_2_7_3.kokkala.txt"} {
		r := load(t, "../ca/testdata/"+f)
		m, ok := FindIso(r, rep, 3)
		if !ok || !SameUpToRows(Apply(r, m), rep) {
			t.Fatalf("%s is not isomorphic to the unique class", f)
		}
	}
	if l := Levels(11, 7, 3); len(l[6]) != 0 {
		t.Fatalf("found a CA(11;2,7,3)")
	}
}

// FindIso and Classify must see through random relabelings.
func TestRandomRelabelings(t *testing.T) {
	r := load(t, "../ca/testdata/CA_12_2_7_3.txt")
	rng := rand.New(rand.NewPCG(1, 2))
	var arrays []Array
	for trial := 0; trial < 50; trial++ {
		k, n := r.K(), r.N()
		colPerm := rng.Perm(k)
		rowPerm := rng.Perm(n)
		x := make(Array, k)
		for c := 0; c < k; c++ {
			sym := rng.Perm(3)
			col := make([]uint8, n)
			for i := 0; i < n; i++ {
				col[rowPerm[i]] = uint8(sym[r[c][i]])
			}
			x[colPerm[c]] = col
		}
		m, ok := FindIso(x, r, 3)
		if !ok || !SameUpToRows(Apply(x, m), r) {
			t.Fatalf("trial %d: relabeled copy not recognized", trial)
		}
		arrays = append(arrays, x)
	}
	if got := len(Classify(arrays, 3)); got != 1 {
		t.Fatalf("50 relabelings gave %d classes, want 1", got)
	}
}

// A non-isomorphic pair must be told apart: change one cell so the array
// stops covering, and FindIso must fail against the original.
func TestRejectsNonIsomorphic(t *testing.T) {
	r := load(t, "../ca/testdata/CA_12_2_7_3.txt")
	x := make(Array, r.K())
	for c := range r {
		x[c] = append([]uint8(nil), r[c]...)
	}
	x[3][5] = (x[3][5] + 1) % 3
	if _, ok := FindIso(x, r, 3); ok {
		t.Fatal("found an isomorphism to a different array")
	}
}
