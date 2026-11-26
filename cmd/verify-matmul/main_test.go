package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestImportsOnlyStdlib(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		if p := strings.Trim(imp.Path.Value, `"`); strings.Contains(p, ".") {
			t.Fatalf("verify-matmul imports %s; it may only use the standard library", p)
		}
	}
}

// Strassen's 7-multiplication scheme for 2x2 matrices, written out by
// hand from the textbook formulas (entries a11 a12 a21 a22, etc.).
//
//	M1 = (a11 + a22)(b11 + b22)   M5 = (a11 + a12) b22
//	M2 = (a21 + a22) b11          M6 = (a21 - a11)(b11 + b12)
//	M3 = a11 (b12 - b22)          M7 = (a12 - a22)(b21 + b22)
//	M4 = a22 (b21 - b11)
//	c11 = M1 + M4 - M5 + M7       c12 = M3 + M5
//	c21 = M2 + M4                 c22 = M1 - M2 + M3 + M6
const strassen = `2 2 2 7
1 0 0 1   1 0 0 1   1 0 0 1
0 0 1 1   1 0 0 0   0 0 1 -1
1 0 0 0   0 1 0 -1  0 1 0 1
0 0 0 1   -1 0 1 0  1 0 1 0
1 1 0 0   0 0 0 1   -1 1 0 0
-1 0 1 0  1 1 0 0   0 0 0 1
0 1 0 -1  0 0 1 1   1 0 0 0
`

func TestStrassen(t *testing.T) {
	s, err := Parse(strassen)
	if err != nil {
		t.Fatal(err)
	}
	if bad, where := Check(s); bad != 0 {
		t.Fatalf("Strassen rejected: %d wrong, %s", bad, where)
	}
}

// The schoolbook scheme for <2,3,4> must pass; every single-entry change
// to Strassen must fail; dropping a term must fail.
func TestRejectsDamage(t *testing.T) {
	s, _ := Parse(strassen)
	for ti := range s.Terms {
		for _, xs := range [][]int64{s.Terms[ti].U, s.Terms[ti].V, s.Terms[ti].W} {
			for e := range xs {
				old := xs[e]
				xs[e] = old + 1
				if bad, _ := Check(s); bad == 0 {
					t.Fatalf("change in term %d not caught", ti)
				}
				xs[e] = old
			}
		}
	}
	short := &Scheme{N: 2, M: 2, P: 2, Terms: s.Terms[:6]}
	if bad, _ := Check(short); bad == 0 {
		t.Fatal("6-term scheme accepted")
	}
}

func TestSchoolbook(t *testing.T) {
	n, m, p := 2, 3, 4
	var b strings.Builder
	b.WriteString("2 3 4 24\n")
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			for k := 0; k < p; k++ {
				row := make([]string, n*m+m*p+n*p)
				for x := range row {
					row[x] = "0"
				}
				row[i*m+j] = "1"
				row[n*m+j*p+k] = "1"
				row[n*m+m*p+i*p+k] = "1"
				b.WriteString(strings.Join(row, " ") + "\n")
			}
		}
	}
	s, err := Parse(b.String())
	if err != nil {
		t.Fatal(err)
	}
	if bad, where := Check(s); bad != 0 {
		t.Fatalf("schoolbook rejected: %s", where)
	}
}

func TestParseRejects(t *testing.T) {
	for _, txt := range []string{"", "2 2 2", "2 2 2 1\n1 0", "2 2 2 2\n" + strings.Repeat("0 ", 12)} {
		if _, err := Parse(txt); err == nil {
			t.Errorf("Parse(%q) should fail", txt)
		}
	}
}
