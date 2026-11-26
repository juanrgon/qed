package lightslice

import (
	"testing"

	"github.com/juanrgon/qed/internal/ca"
)

const K = 8 // the main case: 8 columns, 12-row slices

// Control on a case where an array really exists. The unique
// CA(33; 3, 6, 3) has every column split 11/11/11, so every symbol is
// "light" with 11-row slices. Running the same reduction with k = 6 and
// size 11 must find every real slice as a candidate, and every pair of
// real slices must count as compatible. If the method reported an
// obstruction here, it would be broken.
func TestControlOnRealArray(t *testing.T) {
	big, err := ca.ReadFile("../ca/testdata/CA_33_3_6_3.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !ca.Verify(big, 3, 3).OK() {
		t.Fatal("control array does not verify")
	}
	rows := make([]Row, big.N)
	var A []Row
	for i := 0; i < big.N; i++ {
		rows[i] = pack(big.Row(i))
		if rows[i].At(0) == 0 {
			A = append(A, rows[i])
		}
	}
	if len(A) != 11 {
		t.Fatalf("column 0 symbol 0 has %d rows, want 11", len(A))
	}
	tab := Build(A, 6, 11)
	real := func(Y int, v uint8) Slice {
		var s Slice
		for _, r := range rows {
			if r.At(Y) == v {
				s = append(s, r)
			}
		}
		sortRows(s)
		return s
	}
	for Y := 1; Y < 6; Y++ {
		for v := uint8(0); v < 3; v++ {
			want := string(rowBytes(real(Y, v)))
			found := false
			for _, s := range tab.Cands[Y][v] {
				if string(rowBytes(s)) == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("real slice Y=%d v=%d missing from %d candidates", Y, v, len(tab.Cands[Y][v]))
			}
		}
	}
	for Y := 1; Y < 6; Y++ {
		for Z := Y + 1; Z < 6; Z++ {
			m := tab.Matrix(Y, Z)
			for v := 0; v < 3; v++ {
				for w := 0; w < 3; w++ {
					if m[v][w] == 0 {
						t.Fatalf("false obstruction: columns (%d,%d) symbols (%d,%d) report no compatible pair: %v", Y, Z, v, w, m)
					}
				}
			}
			t.Logf("control (%d,%d): %v", Y, Z, m)
		}
	}
}

func pinned(t *testing.T) []Row {
	t.Helper()
	r, err := ca.ReadFile("../ca/testdata/CA_12_2_7_3.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	return Pinned(r)
}

// Every candidate must really be a light slice: 12 rows, column Y equal
// to v, containing A's rows with Y = v and no other column-0-zero row,
// covering every pair on the other 7 columns (checked by the plain
// verifier, not by the search's own counts).
func TestCandidatesAreValid(t *testing.T) {
	A := pinned(t)
	for Y := 1; Y < K; Y++ {
		for v := uint8(0); v < 3; v++ {
			cands := Completions(A, K, 12, Y, v)
			for _, s := range cands {
				if len(s) != 12 {
					t.Fatalf("Y=%d v=%d: %d rows", Y, v, len(s))
				}
				arr := ca.New(0, 7)
				zeros := 0
				for _, r := range s {
					if r.At(Y) != v {
						t.Fatalf("row with wrong Y value")
					}
					if r.At(0) == 0 {
						zeros++
					}
					var row []uint8
					for c := 0; c < K; c++ {
						if c != Y {
							row = append(row, r.At(c))
						}
					}
					arr.AppendRow(row)
				}
				want := 0
				for _, r := range A {
					if r.At(Y) == v {
						want++
					}
				}
				if zeros != want {
					t.Fatalf("Y=%d v=%d: %d column-0-zero rows, want %d", Y, v, zeros, want)
				}
				if !ca.Verify(arr, 2, 3).OK() {
					t.Fatalf("Y=%d v=%d: candidate is not a CA(12;2,7,3)", Y, v)
				}
			}
		}
	}
}

// Completeness control: build slices by hand that must be found. Take A
// itself relabeled so that column Y plays the role of column 0: for any
// copy of A with the 4 fixed rows inside it and the rest moved to column
// 0 values 1 and 2, the search must find it. The simplest such copy is A
// with its column-0 entries changed only on rows outside the fixed set,
// which is a valid slice exactly when it still covers all pairs; the
// search must agree with the plain verifier on every one of those.
func TestFindsHandBuiltSlices(t *testing.T) {
	A := pinned(t)
	validSeen := 0
	defer func() {
		// The check is only meaningful if some hand-built slices are valid.
		if validSeen == 0 {
			t.Fatal("no valid hand-built slice: completeness control is vacuous")
		}
		t.Logf("%d valid hand-built slices, all found by the search", validSeen)
	}()
	for Y := 1; Y < K; Y++ {
		for v := uint8(0); v < 3; v++ {
			got := map[string]bool{}
			for _, s := range Completions(A, K, 12, Y, v) {
				got[string(rowBytes(s))] = true
			}
			// Brute force a small family: rows of A with Y != v become
			// rows with Y = v and column 0 in {1,2}, keeping the other
			// columns. Try all 2^8 column-0 choices.
			var fixed, rest []Row
			for _, r := range A {
				if r.At(Y) == v {
					fixed = append(fixed, r)
				} else {
					rest = append(rest, r)
				}
			}
			if len(fixed)+len(rest) != 12 {
				continue
			}
			for mask := 0; mask < 1<<len(rest); mask++ {
				s := append(Slice{}, fixed...)
				arr := ca.New(0, 7)
				for i, r := range rest {
					c0 := uint8(1)
					if mask>>i&1 == 1 {
						c0 = 2
					}
					delta := (int(v) - int(r.At(Y))) * pow3[Y]
					delta += (int(c0) - int(r.At(0))) * pow3[0]
					s = append(s, Row(int(r)+delta))
				}
				for _, r := range s {
					var row []uint8
					for c := 0; c < K; c++ {
						if c != Y {
							row = append(row, r.At(c))
						}
					}
					arr.AppendRow(row)
				}
				sortRows(s)
				valid := ca.Verify(arr, 2, 3).OK()
				if valid {
					validSeen++
				}
				if valid != got[string(rowBytes(s))] {
					t.Fatalf("Y=%d v=%d mask=%d: verifier says %v, search says %v", Y, v, mask, valid, !valid)
				}
			}
		}
	}
}

func sortRows(s Slice) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
