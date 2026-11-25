package main

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The judge must not share code with the search.
func TestImportsOnlyStdlib(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		p := strings.Trim(imp.Path.Value, `"`)
		if strings.Contains(p, ".") {
			t.Fatalf("verify-ca imports %s; it may only use the standard library", p)
		}
	}
}

func rowsOf(t *testing.T, path string) [][]int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parse(string(raw), 3)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestKnownArrays(t *testing.T) {
	for _, c := range []struct {
		file string
		t    int
	}{
		{"CA_42_3_8_3.txt", 3},
		{"CA_33_3_6_3.txt", 3},
		{"CA_12_2_7_3.txt", 2},
		{"CA_12_2_7_3.kokkala.txt", 2},
	} {
		rows := rowsOf(t, "../../internal/ca/testdata/"+c.file)
		if m, ex := check(rows, c.t, 3); m != 0 {
			t.Errorf("%s: %d missing, e.g. %s", c.file, m, ex)
		}
	}
}

// Every single-cell change to the 42-row array must break it, and
// deleting any row must too.
func TestRejectsDamage(t *testing.T) {
	rows := rowsOf(t, "../../internal/ca/testdata/CA_42_3_8_3.txt")
	for r := range rows {
		for c := range rows[r] {
			old := rows[r][c]
			rows[r][c] = (old + 1) % 3
			if m, _ := check(rows, 3, 3); m == 0 {
				t.Fatalf("change at row %d col %d not caught", r, c)
			}
			rows[r][c] = old
		}
		less := append(append([][]int{}, rows[:r]...), rows[r+1:]...)
		if m, _ := check(less, 3, 3); m == 0 {
			t.Fatalf("deleting row %d not caught", r)
		}
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	for _, s := range []string{"", "0 1\n0", "0 3", "0 x"} {
		if _, err := parse(s, 3); err == nil {
			t.Errorf("parse(%q) should fail", s)
		}
	}
}
