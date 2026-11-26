package matmul

import (
	"math/rand/v2"
)

// Walker runs a flip graph walk on one scheme. It keeps, for each of the
// three factor positions, an index from a factor (up to sign) to the
// terms that have it, and updates only the terms a move touches.
type Walker struct {
	S      *Scheme
	MaxAbs int8 // largest coefficient a flip may create
	rng    *rand.Rand
	keys   [][3]string
	index  [3]map[string][]int
	buf    Factor
}

// NewWalker prepares a walk from s (which it takes over).
func NewWalker(s *Scheme, maxAbs int8, seed uint64) *Walker {
	w := &Walker{S: s, MaxAbs: maxAbs, rng: rand.New(rand.NewPCG(seed, 0xa5a5))}
	for a := 0; a < 3; a++ {
		w.index[a] = map[string][]int{}
	}
	w.keys = make([][3]string, len(s.Terms))
	for i := range s.Terms {
		w.indexTerm(i)
	}
	w.mergeAll()
	return w
}

// key returns the factor up to sign (first nonzero entry made positive)
// and the sign it took. The zero factor has sign 0.
func key(f Factor) (string, int8) {
	var sign int8
	for _, x := range f {
		if x != 0 {
			if x > 0 {
				sign = 1
			} else {
				sign = -1
			}
			break
		}
	}
	b := make([]byte, len(f))
	for i, x := range f {
		b[i] = byte(x * sign)
	}
	return string(b), sign
}

func sign(f Factor) int8 {
	for _, x := range f {
		if x > 0 {
			return 1
		}
		if x < 0 {
			return -1
		}
	}
	return 0
}

func (w *Walker) indexTerm(i int) {
	for a := 0; a < 3; a++ {
		k, _ := key(w.S.Terms[i][a])
		w.keys[i][a] = k
		w.index[a][k] = append(w.index[a][k], i)
	}
}

func (w *Walker) unindex(i, a int) {
	k := w.keys[i][a]
	b := w.index[a][k]
	for x, id := range b {
		if id == i {
			b[x] = b[len(b)-1]
			b = b[:len(b)-1]
			break
		}
	}
	if len(b) == 0 {
		delete(w.index[a], k)
	} else {
		w.index[a][k] = b
	}
}

func (w *Walker) reindexFactor(i, a int) {
	w.unindex(i, a)
	k, _ := key(w.S.Terms[i][a])
	w.keys[i][a] = k
	w.index[a][k] = append(w.index[a][k], i)
}

// remove deletes term i by moving the last term into its slot.
func (w *Walker) remove(i int) {
	last := len(w.S.Terms) - 1
	for a := 0; a < 3; a++ {
		w.unindex(i, a)
	}
	if i != last {
		for a := 0; a < 3; a++ {
			w.unindex(last, a)
		}
		w.S.Terms[i] = w.S.Terms[last]
		w.keys[i] = w.keys[last]
		for a := 0; a < 3; a++ {
			w.index[a][w.keys[i][a]] = append(w.index[a][w.keys[i][a]], i)
		}
	}
	w.S.Terms = w.S.Terms[:last]
	w.keys = w.keys[:last]
}

// Rank is the current number of multiplications.
func (w *Walker) Rank() int { return len(w.S.Terms) }

func isZero(f Factor) bool {
	for _, x := range f {
		if x != 0 {
			return false
		}
	}
	return true
}

// Flip tries one random flip and returns true if the scheme changed.
//
// If factor a of terms i and j agree up to sign, X_i = s X_j, then for
// t = +-1:
//
//	X_i(x)B_i(x)C_i + X_j(x)B_j(x)C_j
//	  = X_i(x)(B_i + t B_j)(x)C_i + X_j(x)B_j(x)(C_j - s t C_i)
//
// Afterwards zero terms are dropped and terms sharing two factors merged.
func (w *Walker) Flip() bool {
	terms := w.S.Terms
	if len(terms) < 2 {
		return false
	}
	i := w.rng.IntN(len(terms))
	a := w.rng.IntN(3)
	bucket := w.index[a][w.keys[i][a]]
	if len(bucket) < 2 {
		return false
	}
	j := bucket[w.rng.IntN(len(bucket))]
	if j == i {
		return false
	}
	s := sign(terms[i][a]) * sign(terms[j][a])
	b, c := (a+1)%3, (a+2)%3
	if w.rng.IntN(2) == 0 {
		b, c = c, b
	}
	t := int8(1)
	if w.rng.IntN(2) == 0 {
		t = -1
	}
	bi, bj := terms[i][b], terms[j][b]
	ci, cj := terms[i][c], terms[j][c]
	max := int(w.MaxAbs)
	for e := range bi {
		if x := int(bi[e]) + int(t)*int(bj[e]); x > max || x < -max {
			return false
		}
	}
	for e := range cj {
		if x := int(cj[e]) - int(s*t)*int(ci[e]); x > max || x < -max {
			return false
		}
	}
	for e := range bi {
		bi[e] += t * bj[e]
	}
	for e := range cj {
		cj[e] -= s * t * ci[e]
	}
	w.reindexFactor(i, b)
	w.reindexFactor(j, c)
	// Settle j first: removing a term moves the last term into its slot.
	if j > i {
		w.settle(j)
		w.settle(i)
	} else {
		w.settle(i)
		w.settle(j)
	}
	return true
}

// settle drops term i if it has a zero factor, or merges it into another
// term that shares two of its factors (up to sign), repeating on the
// merged term.
func (w *Walker) settle(i int) {
	for i < len(w.S.Terms) {
		t := w.S.Terms[i]
		if isZero(t[0]) || isZero(t[1]) || isZero(t[2]) {
			w.remove(i)
			return
		}
		j, a, b := w.partner(i)
		if j < 0 {
			return
		}
		// t = (sa x, sb y, C_t), u = (ua x, ub y, C_u): fold C_t into u.
		c := 3 - a - b
		u := w.S.Terms[j]
		f := sign(t[a]) * sign(u[a]) * sign(t[b]) * sign(u[b])
		for e := range u[c] {
			v := int(u[c][e]) + int(f)*int(t[c][e])
			if v > 127 || v < -127 {
				return
			}
		}
		for e := range u[c] {
			u[c][e] += f * t[c][e]
		}
		w.reindexFactor(j, c)
		w.remove(i)
		if j == len(w.S.Terms) { // j was the last term and moved into slot i
			j = i
		}
		i = j
	}
}

// partner finds a term sharing two factors with term i.
func (w *Walker) partner(i int) (int, int, int) {
	for a := 0; a < 3; a++ {
		b := (a + 1) % 3
		for _, j := range w.index[a][w.keys[i][a]] {
			if j != i && w.keys[j][b] == w.keys[i][b] {
				return j, a, b
			}
		}
	}
	return -1, 0, 0
}

func (w *Walker) mergeAll() {
	for i := len(w.S.Terms) - 1; i >= 0; i-- {
		if i < len(w.S.Terms) {
			w.settle(i)
		}
	}
}

// Plus makes the scheme one term longer in a way that creates shared
// factors: for terms a(x)b(x)c and a'(x)b'(x)c',
//
//	a(x)b(x)c + a'(x)b'(x)c' = (a - a')(x)b(x)c + a'(x)b(x)c + a'(x)b'(x)c'.
func (w *Walker) Plus() bool {
	terms := w.S.Terms
	if len(terms) < 2 {
		return false
	}
	i, j := w.rng.IntN(len(terms)), w.rng.IntN(len(terms))
	if i == j {
		return false
	}
	pos := w.rng.IntN(3)
	diff := append(Factor(nil), terms[i][pos]...)
	for e := range diff {
		x := int(diff[e]) - int(terms[j][pos][e])
		if x > int(w.MaxAbs) || x < -int(w.MaxAbs) {
			return false
		}
		diff[e] = int8(x)
	}
	if isZero(diff) {
		return false
	}
	var extra Term
	for a := 0; a < 3; a++ {
		extra[a] = append(Factor(nil), terms[i][a]...)
	}
	extra[pos] = append(Factor(nil), terms[j][pos]...)
	w.S.Terms[i][pos] = diff
	w.reindexFactor(i, pos)
	w.S.Terms = append(w.S.Terms, extra)
	w.keys = append(w.keys, [3]string{})
	w.indexTerm(len(w.S.Terms) - 1)
	return true
}
