// Package ca holds covering arrays, a plain exact verifier, and the
// requirement index shared by the search engine and the swap checker.
package ca

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Array is an N x K array of symbols stored row-major.
type Array struct {
	N, K  int
	Cells []uint8
}

// New returns an N x K array filled with zeros.
func New(n, k int) *Array {
	return &Array{N: n, K: k, Cells: make([]uint8, n*k)}
}

func (a *Array) At(r, c int) uint8     { return a.Cells[r*a.K+c] }
func (a *Array) Set(r, c int, v uint8) { a.Cells[r*a.K+c] = v }
func (a *Array) Row(r int) []uint8     { return a.Cells[r*a.K : (r+1)*a.K] }

// Clone returns a deep copy.
func (a *Array) Clone() *Array {
	b := &Array{N: a.N, K: a.K, Cells: make([]uint8, len(a.Cells))}
	copy(b.Cells, a.Cells)
	return b
}

// Without returns a copy with the rows in drop removed.
func (a *Array) Without(drop map[int]bool) *Array {
	b := &Array{K: a.K}
	for r := 0; r < a.N; r++ {
		if !drop[r] {
			b.Cells = append(b.Cells, a.Row(r)...)
			b.N++
		}
	}
	return b
}

// AppendRow adds a row to the end of the array.
func (a *Array) AppendRow(row []uint8) {
	if len(row) != a.K {
		panic("row length mismatch")
	}
	a.Cells = append(a.Cells, row...)
	a.N++
}

// Parse reads whitespace-separated rows. It treats the input strictly as
// data: every row must have the same width and every symbol must be < v.
func Parse(r io.Reader, v int) (*Array, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	a := &Array{}
	line := 0
	for sc.Scan() {
		line++
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		if a.K == 0 {
			a.K = len(f)
		} else if len(f) != a.K {
			return nil, fmt.Errorf("line %d: %d symbols, want %d", line, len(f), a.K)
		}
		for _, s := range f {
			x, err := strconv.Atoi(s)
			if err != nil || x < 0 || x >= v {
				return nil, fmt.Errorf("line %d: bad symbol %q for v=%d", line, s, v)
			}
			a.Cells = append(a.Cells, uint8(x))
		}
		a.N++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if a.N == 0 {
		return nil, fmt.Errorf("empty array")
	}
	return a, nil
}

// ReadFile parses an array file.
func ReadFile(path string, v int) (*Array, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f, v)
}

// Format renders the array as space-separated rows.
func (a *Array) Format() string {
	var b strings.Builder
	for r := 0; r < a.N; r++ {
		for c := 0; c < a.K; c++ {
			if c > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(strconv.Itoa(int(a.At(r, c))))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// WriteFile saves the array in the same format Parse reads.
func (a *Array) WriteFile(path string) error {
	return os.WriteFile(path, []byte(a.Format()), 0o644)
}
