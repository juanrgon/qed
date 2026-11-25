// Command check-lrat checks a text LRAT proof that a DIMACS formula is
// unsatisfiable.
//
// It imports only the Go standard library and shares no code with the
// solver or with internal/sat. It supports the RUP part of LRAT, which is
// what CaDiCaL writes: each added clause lists hint clauses that, in
// order, become unit (or conflicting) once the new clause's literals are
// assumed false. RAT steps (negative hints) are rejected rather than
// trusted. The proof must end by deriving the empty clause.
//
//	check-lrat FORMULA.cnf PROOF.lrat
package main

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: check-lrat FORMULA.cnf PROOF.lrat")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Stdout); err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
}

func run(cnfPath, proofPath string, out io.Writer) error {
	clauses, vars, err := readCNF(cnfPath)
	if err != nil {
		return err
	}
	f, err := os.Open(proofPath)
	if err != nil {
		return err
	}
	defer f.Close()

	db := map[int][]int{}
	maxID := 0
	for i, c := range clauses {
		db[i+1] = c
		maxID = i + 1
	}
	val := make([]int8, vars+1) // 1 true, -1 false, 0 unassigned
	var trail []int
	assign := func(l int) {
		if l > 0 {
			val[l] = 1
		} else {
			val[-l] = -1
		}
		trail = append(trail, l)
	}
	litVal := func(l int) int8 {
		if l > 0 {
			return val[l]
		}
		return -val[-l]
	}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<30)
	line := 0
	empty := false
	for sc.Scan() {
		line++
		toks := strings.Fields(sc.Text())
		if len(toks) == 0 || toks[0] == "c" {
			continue
		}
		id, err := strconv.Atoi(toks[0])
		if err != nil || id <= 0 {
			return fmt.Errorf("line %d: bad clause id", line)
		}
		if len(toks) > 1 && toks[1] == "d" {
			for _, t := range toks[2:] {
				x, err := strconv.Atoi(t)
				if err != nil {
					return fmt.Errorf("line %d: bad deletion", line)
				}
				if x == 0 {
					break
				}
				delete(db, x)
			}
			continue
		}
		if id <= maxID {
			return fmt.Errorf("line %d: clause id %d is not new", line, id)
		}
		// Split "lits 0 hints 0".
		var lits, hints []int
		part := 0
		for _, t := range toks[1:] {
			x, err := strconv.Atoi(t)
			if err != nil {
				return fmt.Errorf("line %d: bad number %q", line, t)
			}
			if x == 0 {
				part++
				if part == 2 {
					break
				}
				continue
			}
			if part == 0 {
				if x > vars || -x > vars {
					return fmt.Errorf("line %d: literal %d out of range", line, x)
				}
				lits = append(lits, x)
			} else {
				hints = append(hints, x)
			}
		}
		if part != 2 {
			return fmt.Errorf("line %d: missing terminating 0", line)
		}
		// RUP check: assume every literal of the new clause false.
		trail = trail[:0]
		ok := false
		conflict := false
		for _, l := range lits {
			switch litVal(l) {
			case 1:
				// l and its negation both in the clause: a tautology.
				ok = true
			case 0:
				assign(-l)
			}
		}
		if !ok {
			for _, h := range hints {
				if h < 0 {
					return fmt.Errorf("line %d: RAT hint %d not supported", line, h)
				}
				c, found := db[h]
				if !found {
					return fmt.Errorf("line %d: hint %d is not an active clause", line, h)
				}
				unassigned := 0
				var unit int
				sat := false
				for _, l := range c {
					switch litVal(l) {
					case 1:
						sat = true
					case 0:
						unassigned++
						unit = l
					}
				}
				if sat {
					return fmt.Errorf("line %d: hint %d is already satisfied", line, h)
				}
				if unassigned == 0 {
					conflict = true
					break
				}
				if unassigned > 1 {
					return fmt.Errorf("line %d: hint %d is not unit", line, h)
				}
				assign(unit)
			}
			ok = conflict
		}
		for _, l := range trail {
			if l > 0 {
				val[l] = 0
			} else {
				val[-l] = 0
			}
		}
		if !ok {
			return fmt.Errorf("line %d: hints for clause %d do not reach a conflict", line, id)
		}
		db[id] = lits
		maxID = id
		if len(lits) == 0 {
			empty = true
			break
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if !empty {
		return fmt.Errorf("proof never derives the empty clause")
	}
	cs, _ := fileSHA(cnfPath)
	ps, _ := fileSHA(proofPath)
	fmt.Fprintf(out, "VERIFIED UNSAT: %d variables, %d clauses, formula sha256=%s proof sha256=%s\n", vars, len(clauses), cs, ps)
	return nil
}

func fileSHA(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(b)), nil
}

func readCNF(path string) ([][]int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<30)
	vars, want := -1, -1
	var clauses [][]int
	var cur []int
	for sc.Scan() {
		toks := strings.Fields(sc.Text())
		if len(toks) == 0 || toks[0] == "c" {
			continue
		}
		if toks[0] == "p" {
			if len(toks) != 4 || toks[1] != "cnf" || vars >= 0 {
				return nil, 0, fmt.Errorf("bad header")
			}
			vars, _ = strconv.Atoi(toks[2])
			want, _ = strconv.Atoi(toks[3])
			continue
		}
		if vars < 0 {
			return nil, 0, fmt.Errorf("clause before header")
		}
		for _, t := range toks {
			x, err := strconv.Atoi(t)
			if err != nil || x > vars || -x > vars {
				return nil, 0, fmt.Errorf("bad literal %q", t)
			}
			if x == 0 {
				clauses = append(clauses, cur)
				cur = nil
			} else {
				cur = append(cur, x)
			}
		}
	}
	if len(cur) != 0 {
		return nil, 0, fmt.Errorf("last clause not terminated")
	}
	if len(clauses) != want {
		return nil, 0, fmt.Errorf("header says %d clauses, found %d", want, len(clauses))
	}
	return clauses, vars, sc.Err()
}
