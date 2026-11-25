package main

import (
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
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
			t.Fatalf("check-lrat imports %s; it may only use the standard library", p)
		}
	}
}

func try(t *testing.T, cnf, proof string) error {
	t.Helper()
	dir := t.TempDir()
	c, p := filepath.Join(dir, "f.cnf"), filepath.Join(dir, "f.lrat")
	os.WriteFile(c, []byte(cnf), 0o644)
	os.WriteFile(p, []byte(proof), 0o644)
	return run(c, p, io.Discard)
}

// (a or b) (a or not b) (not a or b) (not a or not b) is unsatisfiable.
const four = "p cnf 2 4\n1 2 0\n1 -2 0\n-1 2 0\n-1 -2 0\n"

func TestHandProofs(t *testing.T) {
	good := "5 1 0 1 2 0\n6 0 5 3 4 0\n"
	if err := try(t, four, good); err != nil {
		t.Fatalf("good proof rejected: %v", err)
	}
	bad := map[string]string{
		"missing hint":     "5 1 0 1 0\n6 0 5 3 4 0\n",
		"no empty clause":  "5 1 0 1 2 0\n",
		"wrong hint order": "5 1 0 1 2 0\n6 0 3 4 5 0\n",
		"unknown hint":     "5 1 0 1 9 0\n6 0 5 3 4 0\n",
		"deleted hint":     "5 1 0 1 2 0\n5 d 3 0\n6 0 5 3 4 0\n",
		"reused id":        "4 1 0 1 2 0\n6 0 4 3 4 0\n",
		"RAT hint":         "5 1 0 -1 2 0\n6 0 5 3 4 0\n",
		"bad unit step":    "5 2 0 3 0\n6 0 5 3 4 0\n",
	}
	for name, p := range bad {
		if err := try(t, four, p); err == nil {
			t.Errorf("%s: bad proof accepted", name)
		}
	}
	// The same proof must fail against a satisfiable formula.
	if err := try(t, "p cnf 2 4\n1 2 0\n1 -2 0\n-1 2 0\n2 0\n", good); err == nil {
		t.Error("proof accepted against a satisfiable formula")
	}
}
