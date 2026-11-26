package engine

import (
	"math"
	"math/rand/v2"

	"github.com/juanrgon/qed/internal/ca"
)

// Params controls one annealing run. The budget is a move count, not a
// clock, so a run with the same seed always does the same work.
type Params struct {
	Moves  int64
	T0, T1 float64 // geometric cooling from T0 to T1 over the budget
	Sample int     // rows tried per move; the best is proposed
	Noise  float64 // chance of a random single-variable move instead
	Seed   uint64
}

// DefaultParams are a reasonable starting point for small strength-3 arrays.
func DefaultParams() Params {
	return Params{Moves: 50_000_000, T0: 1.0, T1: 0.05, Sample: 4, Noise: 0.02}
}

// Outcome is what one run produced.
type Outcome struct {
	Solved bool
	Best   int   // fewest uncovered requirements seen
	Moves  int64 // moves actually made
	Array  *ca.Array
}

type searcher struct {
	m    *Model
	ix   *ca.Index
	st   *State
	val  []uint8
	rng  *rand.Rand
	V    uint8
	vals [][]uint8 // code -> value tuple
	chX  []int32
	chY  []uint8
	old  []uint8
}

// Anneal runs simulated annealing on model m. Each move picks a random
// uncovered requirement and rewrites the cells of one row so that row
// covers it, tying in every other cell the model links to those cells.
func Anneal(m *Model, ix *ca.Index, p Params) Outcome {
	s := &searcher{m: m, ix: ix, V: uint8(m.V),
		rng: rand.New(rand.NewPCG(p.Seed, 0x9e3779b97f4a7c15))}
	s.vals = make([][]uint8, ix.Pow)
	for code := range s.vals {
		_, s.vals[code] = ix.Decode(code)
	}
	a := ca.New(m.N, m.K)
	s.val = make([]uint8, len(m.Vars))
	for i := range s.val {
		s.val[i] = uint8(s.rng.IntN(m.V))
	}
	for i, owner := range m.Owner {
		if owner < 0 {
			a.Cells[i] = m.Frozen[i]
		} else {
			a.Cells[i] = (s.val[owner] + m.Shift[i]) % s.V
		}
	}
	s.st = NewState(ix, a)

	out := Outcome{Best: s.st.Missing()}
	if p.Sample < 1 {
		p.Sample = 1
	}
	T := p.T0
	ratio := p.T1 / p.T0
	var mv int64
	for mv = 0; mv < p.Moves; mv++ {
		miss := s.st.Missing()
		if miss < out.Best {
			out.Best = miss
		}
		if miss == 0 {
			out.Solved = true
			break
		}
		if mv&1023 == 0 {
			T = p.T0 * math.Pow(ratio, float64(mv)/float64(p.Moves))
		}
		if s.rng.Float64() < p.Noise {
			s.randomMove(T)
			continue
		}
		id := s.st.Uncovered(s.rng.IntN(miss))
		ci, code := id/ix.Pow, id%ix.Pow
		cols, want := ix.Combos[ci], s.vals[code]

		bestRow, bestD, ties := -1, math.MaxInt, 0
		if p.Sample == 1 {
			bestRow = s.rng.IntN(m.N)
		} else {
			for k := 0; k < p.Sample; k++ {
				r := s.rng.IntN(m.N)
				if !s.plan(r, cols, want) {
					continue
				}
				d := s.applyPlan()
				s.revertPlan()
				switch {
				case d < bestD:
					bestRow, bestD, ties = r, d, 1
				case d == bestD:
					ties++
					if s.rng.IntN(ties) == 0 {
						bestRow = r
					}
				}
			}
		}
		if bestRow < 0 || !s.plan(bestRow, cols, want) {
			continue
		}
		d := s.applyPlan()
		if d > 0 && s.rng.Float64() >= math.Exp(-float64(d)/T) {
			s.revertPlan()
		}
	}
	out.Moves = mv
	if s.st.Missing() < out.Best {
		out.Best = s.st.Missing()
	}
	if s.st.Missing() == 0 {
		out.Solved = true
		out.Array = a.Clone()
	}
	return out
}

// plan fills the change buffer with the variable writes that make row r
// show want on cols. It returns false if a frozen cell or a tied cell
// makes that impossible.
func (s *searcher) plan(r int, cols []int, want []uint8) bool {
	s.chX, s.chY = s.chX[:0], s.chY[:0]
	for j, c := range cols {
		i := r*s.m.K + c
		owner := s.m.Owner[i]
		if owner < 0 {
			if s.m.Frozen[i] != want[j] {
				return false
			}
			continue
		}
		y := (want[j] + s.V - s.m.Shift[i]) % s.V
		dup := false
		for k, x := range s.chX {
			if x == owner {
				if s.chY[k] != y {
					return false
				}
				dup = true
			}
		}
		if !dup && s.val[owner] != y {
			s.chX = append(s.chX, owner)
			s.chY = append(s.chY, y)
		}
	}
	return true
}

func (s *searcher) setVar(x int32, y uint8) int {
	d := 0
	for _, im := range s.m.Vars[x] {
		d += s.st.Set(int(im.Row), int(im.Col), (y+im.Shift)%s.V)
	}
	s.val[x] = y
	return d
}

func (s *searcher) applyPlan() int {
	s.old = s.old[:0]
	d := 0
	for k, x := range s.chX {
		s.old = append(s.old, s.val[x])
		d += s.setVar(x, s.chY[k])
	}
	return d
}

func (s *searcher) revertPlan() {
	for k := len(s.chX) - 1; k >= 0; k-- {
		s.setVar(s.chX[k], s.old[k])
	}
}

func (s *searcher) randomMove(T float64) {
	if len(s.val) == 0 {
		return
	}
	x := int32(s.rng.IntN(len(s.val)))
	old := s.val[x]
	y := (old + 1 + uint8(s.rng.IntN(int(s.V)-1))) % s.V
	d := s.setVar(x, y)
	if d > 0 && s.rng.Float64() >= math.Exp(-float64(d)/T) {
		s.setVar(x, old)
	}
}
