package ca

// Index numbers every requirement (a column tuple plus a value tuple) and
// records, for each column, which tuples contain it and with what weight.
// The engine and the swap checker use it; Verify does not.
type Index struct {
	T, K, V int
	Pow     int     // V^T
	Combos  [][]int // column tuples
	ByCol   [][]ColRef
}

// ColRef says column c appears in combo Combo, contributing value*Weight
// to that combo's code.
type ColRef struct {
	Combo  int32
	Weight int32
}

// NewIndex builds the index for strength t, k columns, v symbols.
func NewIndex(t, k, v int) *Index {
	ix := &Index{T: t, K: k, V: v, Pow: 1, Combos: Combinations(k, t)}
	for i := 0; i < t; i++ {
		ix.Pow *= v
	}
	ix.ByCol = make([][]ColRef, k)
	for ci, cols := range ix.Combos {
		w := 1
		for j := t - 1; j >= 0; j-- {
			ix.ByCol[cols[j]] = append(ix.ByCol[cols[j]], ColRef{Combo: int32(ci), Weight: int32(w)})
			w *= v
		}
	}
	return ix
}

// NumReq is the total number of requirements.
func (ix *Index) NumReq() int { return len(ix.Combos) * ix.Pow }

// Code is the value-tuple code of row on combo ci.
func (ix *Index) Code(row []uint8, ci int) int {
	code := 0
	for _, c := range ix.Combos[ci] {
		code = code*ix.V + int(row[c])
	}
	return code
}

// Decode returns the columns and values of requirement id.
func (ix *Index) Decode(id int) ([]int, []uint8) {
	ci, code := id/ix.Pow, id%ix.Pow
	vals := make([]uint8, ix.T)
	for j := ix.T - 1; j >= 0; j-- {
		vals[j] = uint8(code % ix.V)
		code /= ix.V
	}
	return ix.Combos[ci], vals
}
