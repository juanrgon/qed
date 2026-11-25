// Package sat builds CNF formulas and runs CaDiCaL on them.
//
// An UNSAT answer only counts with a proof. Solve asks CaDiCaL for a text
// LRAT proof, and cmd/check-lrat (standard library only, no code shared
// with this package or the solver) checks it. A SAT answer only counts
// once the caller decodes the model and runs it through the domain's own
// verifier.
package sat

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// CNF is a formula in conjunctive normal form over variables 1..Vars.
type CNF struct {
	Vars    int
	Clauses [][]int
}

// NewVar returns a fresh variable.
func (f *CNF) NewVar() int {
	f.Vars++
	return f.Vars
}

// Add appends one clause.
func (f *CNF) Add(lits ...int) {
	f.Clauses = append(f.Clauses, append([]int(nil), lits...))
}

// AtLeast adds clauses forcing at least k of xs to be true, with a
// sequential counter (Sinz 2005) on the negated literals: at most
// len(xs)-k of them may be false.
func (f *CNF) AtLeast(xs []int, k int) {
	neg := make([]int, len(xs))
	for i, x := range xs {
		neg[i] = -x
	}
	f.AtMost(neg, len(xs)-k)
}

// AtMost adds clauses forcing at most k of xs to be true (sequential
// counter: s[i][j] means "at least j+1 of xs[0..i] are true").
func (f *CNF) AtMost(xs []int, k int) {
	n := len(xs)
	if k >= n {
		return
	}
	if k < 0 {
		f.Add() // empty clause: unsatisfiable
		return
	}
	if k == 0 {
		for _, x := range xs {
			f.Add(-x)
		}
		return
	}
	s := make([][]int, n)
	for i := range s {
		s[i] = make([]int, k)
		for j := range s[i] {
			s[i][j] = f.NewVar()
		}
	}
	f.Add(-xs[0], s[0][0])
	for j := 1; j < k; j++ {
		f.Add(-s[0][j])
	}
	for i := 1; i < n; i++ {
		f.Add(-xs[i], s[i][0])
		f.Add(-s[i-1][0], s[i][0])
		for j := 1; j < k; j++ {
			f.Add(-xs[i], -s[i-1][j-1], s[i][j])
			f.Add(-s[i-1][j], s[i][j])
		}
		f.Add(-xs[i], -s[i-1][k-1])
	}
}

// WriteDIMACS writes the formula in DIMACS format.
func (f *CNF) WriteDIMACS(path string) error {
	var b bytes.Buffer
	fmt.Fprintf(&b, "p cnf %d %d\n", f.Vars, len(f.Clauses))
	for _, c := range f.Clauses {
		for _, l := range c {
			b.WriteString(strconv.Itoa(l))
			b.WriteByte(' ')
		}
		b.WriteString("0\n")
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// Result is a solver answer. For SAT, Model[v] is the value of variable v.
// For UNSAT, the proof is at the path given to Solve.
type Result struct {
	Sat     bool
	Unknown bool // the conflict limit was hit (SolveLimit only)
	Model   []bool
}

// ErrNoSolver means CaDiCaL is not installed.
var ErrNoSolver = errors.New("cadical not found (brew install cadical)")

// CakeLPR finds the cake_lpr proof checker (a formally verified LRAT
// checker built with CakeML): $MATHBENCH_CAKE_LPR, then PATH, then
// ~/github.com/tanyongkiam/cake_lpr/cake_lpr.
func CakeLPR() (string, bool) {
	if p := os.Getenv("MATHBENCH_CAKE_LPR"); p != "" {
		return p, true
	}
	if p, err := exec.LookPath("cake_lpr"); err == nil {
		return p, true
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := home + "/github.com/tanyongkiam/cake_lpr/cake_lpr"
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// CheckCake runs cake_lpr on a formula and proof. cake_lpr exits 0 even
// when a proof fails, so success is only the line "s VERIFIED UNSAT".
func CheckCake(bin, cnfPath, proofPath string) error {
	out, err := exec.Command(bin, cnfPath, proofPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("cake_lpr: %v: %s", err, bytes.TrimSpace(out))
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(line) == "s VERIFIED UNSAT" {
			return nil
		}
	}
	return fmt.Errorf("cake_lpr rejected the proof: %s", bytes.TrimSpace(out))
}

// SolveFast is Solve without a proof, for search loops where an UNSAT
// answer is only a hint and never becomes a claim.
func SolveFast(f *CNF, cnfPath string) (Result, error) {
	return solve(f, cnfPath, "", 0)
}

// SolveLimit is SolveFast with a cap on solver conflicts. If the cap is
// hit, the result has Unknown set.
func SolveLimit(f *CNF, cnfPath string, conflicts int) (Result, error) {
	return solve(f, cnfPath, "", conflicts)
}

// Solve writes f to cnfPath, runs CaDiCaL, and for UNSAT leaves a text
// LRAT proof at proofPath (no proof if proofPath is empty).
func Solve(f *CNF, cnfPath, proofPath string) (Result, error) {
	return solve(f, cnfPath, proofPath, 0)
}

func solve(f *CNF, cnfPath, proofPath string, conflicts int) (Result, error) {
	bin, err := exec.LookPath("cadical")
	if err != nil {
		return Result{}, ErrNoSolver
	}
	if err := f.WriteDIMACS(cnfPath); err != nil {
		return Result{}, err
	}
	args := []string{"-q"}
	if conflicts > 0 {
		args = append(args, "-c", strconv.Itoa(conflicts))
	}
	if proofPath != "" {
		args = append(args, "--lrat=true", "--binary=false", cnfPath, proofPath)
	} else {
		args = append(args, cnfPath)
	}
	cmd := exec.Command(bin, args...)
	out, err := cmd.Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		return Result{}, err
	}
	switch code {
	case 10:
		model := make([]bool, f.Vars+1)
		sc := bufio.NewScanner(bytes.NewReader(out))
		sc.Buffer(make([]byte, 1<<20), 1<<26)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "v ") {
				continue
			}
			for _, tok := range strings.Fields(line[2:]) {
				x, err := strconv.Atoi(tok)
				if err != nil {
					return Result{}, fmt.Errorf("bad model token %q", tok)
				}
				if x > 0 && x <= f.Vars {
					model[x] = true
				}
			}
		}
		return Result{Sat: true, Model: model}, nil
	case 20:
		return Result{Sat: false}, nil
	case 0:
		if conflicts > 0 {
			return Result{Unknown: true}, nil
		}
	}
	return Result{}, fmt.Errorf("cadical exit code %d", code)
}
