// Package engine is the fast search core: exact incremental coverage
// counts, a model that maps search variables onto array cells (so a
// strategy can impose structure such as symmetry or frozen columns), and
// a simulated annealing search driven by uncovered requirements.
package engine

import "github.com/juanrgon/mathbench/internal/ca"

// State tracks how many rows cover each requirement and keeps the set of
// uncovered requirements so a random one can be drawn in O(1).
type State struct {
	ix  *ca.Index
	A   *ca.Array
	cnt []uint16
	unc []int32  // uncovered requirement ids
	pos []int32  // position in unc, or -1
	t3  [][]ref3 // strength-3 fast path, per column
}

// ref3 precomputes, for column c in one strength-3 combo, the combo's
// base id and the other two columns with their code weights.
type ref3 struct {
	base       int32
	w1, w2, wc int32
	o1, o2     int32
}

// NewState builds counts for a from scratch.
func NewState(ix *ca.Index, a *ca.Array) *State {
	s := &State{ix: ix, A: a, cnt: make([]uint16, ix.NumReq()), pos: make([]int32, ix.NumReq())}
	if ix.T == 3 {
		s.t3 = make([][]ref3, ix.K)
		for c := 0; c < ix.K; c++ {
			for _, ref := range ix.ByCol[c] {
				cols := ix.Combos[ref.Combo]
				r := ref3{base: ref.Combo * int32(ix.Pow), wc: ref.Weight, o1: -1}
				w := int32(ix.Pow)
				for _, col := range cols {
					w /= int32(ix.V)
					if col == c {
						continue
					}
					if r.o1 < 0 {
						r.o1, r.w1 = int32(col), w
					} else {
						r.o2, r.w2 = int32(col), w
					}
				}
				s.t3[c] = append(s.t3[c], r)
			}
		}
	}
	for r := 0; r < a.N; r++ {
		row := a.Row(r)
		for ci := range ix.Combos {
			s.cnt[ci*ix.Pow+ix.Code(row, ci)]++
		}
	}
	for id := range s.cnt {
		s.pos[id] = -1
		if s.cnt[id] == 0 {
			s.pos[id] = int32(len(s.unc))
			s.unc = append(s.unc, int32(id))
		}
	}
	return s
}

// Missing is the number of uncovered requirements.
func (s *State) Missing() int { return len(s.unc) }

// Uncovered returns the i-th uncovered requirement id.
func (s *State) Uncovered(i int) int { return int(s.unc[i]) }

// Set changes one cell and returns the change in Missing.
func (s *State) Set(r, c int, v uint8) int {
	row := s.A.Row(r)
	old := row[c]
	if old == v {
		return 0
	}
	if s.t3 != nil {
		return s.set3(row, c, old, v)
	}
	diff := int(v) - int(old)
	delta := 0
	pow := s.ix.Pow
	for _, ref := range s.ix.ByCol[c] {
		ci := int(ref.Combo)
		code := 0
		for _, col := range s.ix.Combos[ci] {
			code = code*s.ix.V + int(row[col])
		}
		from := ci*pow + code
		to := from + diff*int(ref.Weight)
		s.cnt[from]--
		if s.cnt[from] == 0 {
			s.pos[from] = int32(len(s.unc))
			s.unc = append(s.unc, int32(from))
			delta++
		}
		s.cnt[to]++
		if s.cnt[to] == 1 {
			p := s.pos[to]
			last := s.unc[len(s.unc)-1]
			s.unc[p] = last
			s.pos[last] = p
			s.unc = s.unc[:len(s.unc)-1]
			s.pos[to] = -1
			delta--
		}
	}
	row[c] = v
	return delta
}

func (s *State) set3(row []uint8, c int, old, v uint8) int {
	delta := 0
	cnt := s.cnt
	for _, r := range s.t3[c] {
		from := r.base + int32(row[r.o1])*r.w1 + int32(row[r.o2])*r.w2 + int32(old)*r.wc
		to := from + (int32(v)-int32(old))*r.wc
		cnt[from]--
		if cnt[from] == 0 {
			s.pos[from] = int32(len(s.unc))
			s.unc = append(s.unc, from)
			delta++
		}
		cnt[to]++
		if cnt[to] == 1 {
			p := s.pos[to]
			last := s.unc[len(s.unc)-1]
			s.unc[p] = last
			s.pos[last] = p
			s.unc = s.unc[:len(s.unc)-1]
			s.pos[to] = -1
			delta--
		}
	}
	row[c] = v
	return delta
}
