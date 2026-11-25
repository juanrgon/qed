package engine

import "fmt"

// Image is one array cell a variable controls. The cell's value is the
// variable's value plus Shift, mod v.
type Image struct {
	Row, Col int32
	Shift    uint8
}

// Model maps search variables to array cells. Structure lives here: a
// free model gives every cell its own variable; a symmetric model ties
// cells in an orbit to one variable; frozen cells have no variable.
type Model struct {
	Name    string
	N, K, V int
	Vars    [][]Image // cells controlled by each variable
	Owner   []int32   // per cell: variable index, or -1 if frozen
	Shift   []uint8   // per cell: shift from its variable
	Frozen  []uint8   // per cell: fixed value when Owner is -1
}

func newModel(name string, n, k, v int) *Model {
	m := &Model{Name: name, N: n, K: k, V: v,
		Owner: make([]int32, n*k), Shift: make([]uint8, n*k), Frozen: make([]uint8, n*k)}
	for i := range m.Owner {
		m.Owner[i] = -1
	}
	return m
}

func (m *Model) addVar(cells ...Image) {
	id := int32(len(m.Vars))
	for _, im := range cells {
		i := int(im.Row)*m.K + int(im.Col)
		if m.Owner[i] != -1 {
			panic("cell owned twice")
		}
		m.Owner[i] = id
		m.Shift[i] = im.Shift
	}
	m.Vars = append(m.Vars, cells)
}

// Free gives every cell its own variable.
func Free(n, k, v int) *Model {
	m := newModel(fmt.Sprintf("free(N=%d)", n), n, k, v)
	for r := 0; r < n; r++ {
		for c := 0; c < k; c++ {
			m.addVar(Image{Row: int32(r), Col: int32(c)})
		}
	}
	return m
}

// SplitColumn0 freezes column 0 so symbol s appears exactly split[s]
// times, and leaves every other cell free. Every covering array can be
// relabeled to match one split, so this is a restriction without loss.
func SplitColumn0(k, v int, split []int) *Model {
	n := 0
	for _, x := range split {
		n += x
	}
	m := newModel(fmt.Sprintf("split%v", split), n, k, v)
	r := 0
	for s, x := range split {
		for i := 0; i < x; i++ {
			m.Frozen[r*k] = uint8(s)
			for c := 1; c < k; c++ {
				m.addVar(Image{Row: int32(r), Col: int32(c)})
			}
			r++
		}
	}
	return m
}

// ShiftOrbits builds reps orbits under the symbol shift x -> x+1 mod v
// (each orbit is v rows), plus free rows with no symmetry. The published
// CA(42;3,8,3) is exactly 14 such orbits.
func ShiftOrbits(reps, free, k, v int) *Model {
	n := reps*v + free
	m := newModel(fmt.Sprintf("shift(reps=%d,free=%d)", reps, free), n, k, v)
	for i := 0; i < reps; i++ {
		for c := 0; c < k; c++ {
			cells := make([]Image, v)
			for s := 0; s < v; s++ {
				cells[s] = Image{Row: int32(i*v + s), Col: int32(c), Shift: uint8(s)}
			}
			m.addVar(cells...)
		}
	}
	for r := reps * v; r < n; r++ {
		for c := 0; c < k; c++ {
			m.addVar(Image{Row: int32(r), Col: int32(c)})
		}
	}
	return m
}
