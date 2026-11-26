package matmul

import "testing"

func TestStandardIsCorrect(t *testing.T) {
	for _, d := range [][3]int{{1, 1, 1}, {2, 2, 2}, {2, 3, 4}, {3, 3, 3}} {
		s := Standard(d[0], d[1], d[2])
		if !s.Correct() || len(s.Terms) != d[0]*d[1]*d[2] {
			t.Fatalf("standard %v is wrong", d)
		}
	}
}

// Every flip and plus move must keep the scheme correct.
func TestMovesKeepCorrectness(t *testing.T) {
	w := NewWalker(Standard(3, 3, 3), 2, 1)
	for step := 0; step < 3000; step++ {
		if step%50 == 49 {
			w.Plus()
		} else {
			w.Flip()
		}
		if !w.S.Correct() {
			t.Fatalf("step %d broke the scheme (rank %d)", step, w.Rank())
		}
	}
}

// Control: the walk must rediscover Strassen's rank 7 for 2x2 matrices.
func TestFindsStrassenRank(t *testing.T) {
	for seed := uint64(0); seed < 4; seed++ {
		res := Search(Standard(2, 2, 2), Params{Steps: 200000, MaxAbs: 1, Stall: 2000, Seed: seed, Target: 7, Restart: 20000}, nil)
		if len(res.Best.Terms) != 7 || !res.Best.Correct() {
			t.Fatalf("seed %d: best rank %d", seed, len(res.Best.Terms))
		}
	}
}
