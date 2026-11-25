package ca

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

const witnessSHA = "d92e84fcc068215ef476d98c960d97d9e3a17c1cdb289394e37cf232b23eb5c9"

func loadWitness(t *testing.T) *Array {
	t.Helper()
	raw, err := os.ReadFile("testdata/CA_42_3_8_3.txt")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if got := hex.EncodeToString(sum[:]); got != witnessSHA {
		t.Fatalf("witness sha256 = %s, want %s", got, witnessSHA)
	}
	a, err := ReadFile("testdata/CA_42_3_8_3.txt", 3)
	if err != nil {
		t.Fatal(err)
	}
	if a.N != 42 || a.K != 8 {
		t.Fatalf("got %dx%d, want 42x8", a.N, a.K)
	}
	return a
}

func TestPublishedWitnessVerifies(t *testing.T) {
	a := loadWitness(t)
	if rep := Verify(a, 3, 3); !rep.OK() {
		t.Fatalf("published CA(42;3,8,3) reported %d missing", rep.Missing)
	}
}

func TestAnySingleCellChangeFails(t *testing.T) {
	a := loadWitness(t)
	for r := 0; r < a.N; r++ {
		for c := 0; c < a.K; c++ {
			old := a.At(r, c)
			for v := uint8(0); v < 3; v++ {
				if v == old {
					continue
				}
				a.Set(r, c, v)
				if Verify(a, 3, 3).OK() {
					t.Fatalf("changing (%d,%d) to %d still verified", r, c, v)
				}
				a.Set(r, c, old)
			}
		}
	}
}

// The panel said deleting any one row uncovers exactly 18 or 24 triples.
func TestDeleteOneRowUncovers18Or24(t *testing.T) {
	a := loadWitness(t)
	for r := 0; r < a.N; r++ {
		m := Verify(a.Without(map[int]bool{r: true}), 3, 3).Missing
		if m != 18 && m != 24 {
			t.Fatalf("deleting row %d uncovers %d", r, m)
		}
	}
}

func TestParseRejectsBadData(t *testing.T) {
	for _, in := range []string{"0 1\n2", "0 3\n", "x 0\n", ""} {
		if _, err := Parse(stringsReader(in), 3); err == nil {
			t.Errorf("Parse(%q) accepted bad input", in)
		}
	}
}
