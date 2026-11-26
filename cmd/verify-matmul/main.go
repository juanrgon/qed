// Command verify-matmul is the standalone judge for matrix multiplication
// schemes. It imports only the Go standard library.
//
// A scheme multiplies an n x m matrix A by an m x p matrix B using r
// multiplications. Term t has three integer matrices: U_t (n x m),
// V_t (m x p) and W_t (n x p). It computes
//
//	M_t = (sum of U_t[i][j] * A[i][j]) * (sum of V_t[j][k] * B[j][k])
//
// and sets C[i][k] = sum over t of W_t[i][k] * M_t. The scheme is correct
// exactly when, for all indices,
//
//	sum_t U_t[i][j] * V_t[j2][k] * W_t[i2][k2] = [j == j2][i == i2][k == k2]
//
// (the Brent equations). This program checks every one of them in exact
// integer arithmetic, so a correct scheme works over any ring.
//
// File format: a first line "n m p r", then r lines, each with n*m + m*p
// + n*p integers: U row by row, then V, then W. Blank lines and lines
// starting with # are ignored.
//
//	verify-matmul FILE
package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: verify-matmul FILE")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	s, err := Parse(string(raw))
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	if bad, where := Check(s); bad > 0 {
		fmt.Printf("FAIL: %d Brent equations wrong, e.g. %s\n", bad, where)
		os.Exit(1)
	}
	fmt.Printf("VERIFIED: <%d,%d,%d> with %d multiplications, max |coefficient| %d, sha256=%x\n",
		s.N, s.M, s.P, len(s.Terms), s.MaxCoef(), sha256.Sum256(raw))
}

// Scheme is a parsed scheme.
type Scheme struct {
	N, M, P int
	Terms   []Term
}

// Term holds U (N*M), V (M*P), W (N*P), row-major.
type Term struct{ U, V, W []int64 }

// MaxCoef is the largest absolute coefficient.
func (s *Scheme) MaxCoef() int64 {
	var m int64
	for _, t := range s.Terms {
		for _, xs := range [][]int64{t.U, t.V, t.W} {
			for _, x := range xs {
				if x < 0 {
					x = -x
				}
				if x > m {
					m = x
				}
			}
		}
	}
	return m
}

// Parse reads the file format described above.
func Parse(text string) (*Scheme, error) {
	var nums [][]int64
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var row []int64
		for _, f := range strings.Fields(line) {
			x, err := strconv.ParseInt(f, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("bad number %q", f)
			}
			if x > 1<<20 || x < -(1<<20) {
				return nil, fmt.Errorf("coefficient %d too large", x)
			}
			row = append(row, x)
		}
		nums = append(nums, row)
	}
	if len(nums) == 0 || len(nums[0]) != 4 {
		return nil, fmt.Errorf("first line must be \"n m p r\"")
	}
	h := nums[0]
	n, m, p, r := int(h[0]), int(h[1]), int(h[2]), int(h[3])
	if n < 1 || m < 1 || p < 1 || n > 32 || m > 32 || p > 32 || r < 1 {
		return nil, fmt.Errorf("bad header %v", h)
	}
	if len(nums)-1 != r {
		return nil, fmt.Errorf("header says %d terms, found %d", r, len(nums)-1)
	}
	s := &Scheme{N: n, M: m, P: p}
	for i, row := range nums[1:] {
		if len(row) != n*m+m*p+n*p {
			return nil, fmt.Errorf("term %d has %d numbers, want %d", i+1, len(row), n*m+m*p+n*p)
		}
		s.Terms = append(s.Terms, Term{U: row[:n*m], V: row[n*m : n*m+m*p], W: row[n*m+m*p:]})
	}
	return s, nil
}

// Check counts the Brent equations that fail and describes one.
func Check(s *Scheme) (int, string) {
	n, m, p := s.N, s.M, s.P
	bad := 0
	where := ""
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			for j2 := 0; j2 < m; j2++ {
				for k := 0; k < p; k++ {
					for i2 := 0; i2 < n; i2++ {
						for k2 := 0; k2 < p; k2++ {
							var sum int64
							for _, t := range s.Terms {
								sum += t.U[i*m+j] * t.V[j2*p+k] * t.W[i2*p+k2]
							}
							var want int64
							if j == j2 && i == i2 && k == k2 {
								want = 1
							}
							if sum != want {
								bad++
								if where == "" {
									where = fmt.Sprintf("A[%d][%d]*B[%d][%d] in C[%d][%d]: got %d, want %d", i, j, j2, k, i2, k2, sum, want)
								}
							}
						}
					}
				}
			}
		}
	}
	return bad, where
}
