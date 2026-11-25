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
//
// Watch out: the shift moves every requirement to another one on the same
// columns, and one row covers at most one of those. So a gap left by the
// orbits needs v free rows to fix, and fewer than v free rows add nothing.
func ShiftOrbits(reps, free, k, v int) *Model {
	g, _ := Group("z3", k, v)
	return Orbits(reps, free, k, v, "shift", g)
}

// Elem is one group element: it sends column c to Perm[c] and adds Shift
// to every symbol.
type Elem struct {
	Perm  []int
	Shift uint8
}

// Group returns a named group acting on k columns and v symbols:
//
//	z3    symbol shifts only (order v)
//	c7    cycle columns 1..k-1, column 0 fixed (order k-1)
//	ck    cycle all k columns (order k)
//	c7z3  c7 combined with symbol shifts (order (k-1)*v)
//	ckz3  ck combined with symbol shifts (order k*v)
func Group(name string, k, v int) ([]Elem, error) {
	cyc := func(fix0 bool) [][]int {
		n := k
		if fix0 {
			n = k - 1
		}
		var perms [][]int
		for g := 0; g < n; g++ {
			p := make([]int, k)
			for c := 0; c < k; c++ {
				switch {
				case fix0 && c == 0:
					p[c] = 0
				case fix0:
					p[c] = 1 + (c-1+g)%n
				default:
					p[c] = (c + g) % n
				}
			}
			perms = append(perms, p)
		}
		return perms
	}
	id := make([]int, k)
	for c := range id {
		id[c] = c
	}
	var perms [][]int
	shifts := 1
	switch name {
	case "z3":
		perms, shifts = [][]int{id}, v
	case "c7":
		perms = cyc(true)
	case "ck":
		perms = cyc(false)
	case "c7z3":
		perms, shifts = cyc(true), v
	case "ckz3":
		perms, shifts = cyc(false), v
	default:
		return nil, fmt.Errorf("unknown group %q", name)
	}
	var out []Elem
	for _, p := range perms {
		for s := 0; s < shifts; s++ {
			out = append(out, Elem{Perm: p, Shift: uint8(s)})
		}
	}
	return out, nil
}

// Orbits builds reps orbits of rows under group g (each orbit is len(g)
// rows), plus free rows with no symmetry.
func Orbits(reps, free, k, v int, name string, g []Elem) *Model {
	n := reps*len(g) + free
	m := newModel(fmt.Sprintf("%s(reps=%d,free=%d)", name, reps, free), n, k, v)
	for i := 0; i < reps; i++ {
		for c := 0; c < k; c++ {
			cells := make([]Image, len(g))
			for j, e := range g {
				cells[j] = Image{Row: int32(i*len(g) + j), Col: int32(e.Perm[c]), Shift: e.Shift}
			}
			m.addVar(cells...)
		}
	}
	for r := reps * len(g); r < n; r++ {
		for c := 0; c < k; c++ {
			m.addVar(Image{Row: int32(r), Col: int32(c)})
		}
	}
	return m
}
