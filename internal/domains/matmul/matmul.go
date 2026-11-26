// Package matmul searches for matrix multiplication schemes with few
// multiplications, using a flip graph walk over the integers.
//
// A scheme for <n,m,p> is a list of terms (U, V, W): U is n x m, V is
// m x p, W is n x p, all integer. It is correct when the sum over terms
// of U (x) V (x) W equals the matrix multiplication tensor (see
// cmd/verify-matmul, the standalone judge).
//
// The walk (after Kauers and Moosbauer, done here over Z rather than
// GF(2), so every scheme found is correct over any ring with no lifting
// step): if two terms share a factor, say U_i = U_j = X, then
//
//	X(x)V_i(x)W_i + X(x)V_j(x)W_j = X(x)(V_i + V_j)(x)W_i + X(x)V_j(x)(W_j - W_i)
//
// so the scheme stays correct and keeps its size ("flip"). If a factor
// becomes zero, that term is dropped; if two terms come to share two
// factors, they merge. Both make the scheme one multiplication shorter.
// When the walk stalls, a "plus" move makes it one longer in a way that
// creates new shared factors.
package matmul

import (
	"fmt"
	"strconv"
	"strings"
)

// Factor is a flattened integer matrix.
type Factor []int8

// Term is one multiplication.
type Term [3]Factor // U, V, W

// Scheme is a list of terms for <N,M,P>.
type Scheme struct {
	N, M, P int
	Terms   []Term
}

// Sizes returns the lengths of U, V and W.
func (s *Scheme) Sizes() [3]int { return [3]int{s.N * s.M, s.M * s.P, s.N * s.P} }

// Standard returns the schoolbook scheme with N*M*P terms.
func Standard(n, m, p int) *Scheme {
	s := &Scheme{N: n, M: m, P: p}
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			for k := 0; k < p; k++ {
				u, v, w := make(Factor, n*m), make(Factor, m*p), make(Factor, n*p)
				u[i*m+j], v[j*p+k], w[i*p+k] = 1, 1, 1
				s.Terms = append(s.Terms, Term{u, v, w})
			}
		}
	}
	return s
}

// Clone deep-copies s.
func (s *Scheme) Clone() *Scheme {
	c := &Scheme{N: s.N, M: s.M, P: s.P, Terms: make([]Term, len(s.Terms))}
	for i, t := range s.Terms {
		for a := 0; a < 3; a++ {
			c.Terms[i][a] = append(Factor(nil), t[a]...)
		}
	}
	return c
}

// Correct checks the Brent equations (a plain check for tests and for
// catching search bugs early; the judge is cmd/verify-matmul).
func (s *Scheme) Correct() bool {
	n, m, p := s.N, s.M, s.P
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			for j2 := 0; j2 < m; j2++ {
				for k := 0; k < p; k++ {
					for i2 := 0; i2 < n; i2++ {
						for k2 := 0; k2 < p; k2++ {
							sum := 0
							for _, t := range s.Terms {
								sum += int(t[0][i*m+j]) * int(t[1][j2*p+k]) * int(t[2][i2*p+k2])
							}
							want := 0
							if j == j2 && i == i2 && k == k2 {
								want = 1
							}
							if sum != want {
								return false
							}
						}
					}
				}
			}
		}
	}
	return true
}

// String writes the verify-matmul file format.
func (s *Scheme) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %d %d %d\n", s.N, s.M, s.P, len(s.Terms))
	for _, t := range s.Terms {
		var parts []string
		for a := 0; a < 3; a++ {
			for _, x := range t[a] {
				parts = append(parts, strconv.Itoa(int(x)))
			}
		}
		b.WriteString(strings.Join(parts, " ") + "\n")
	}
	return b.String()
}
