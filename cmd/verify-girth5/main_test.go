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
			t.Fatalf("verify-girth5 imports %s; it may only use the standard library", p)
		}
	}
}

// The Petersen graph in graph6 is "IheA@GUAo" (a standard listing).
func TestPetersen(t *testing.T) {
	adj, err := ParseGraph6("IheA@GUAo")
	if err != nil {
		t.Fatal(err)
	}
	if len(adj) != 10 || Edges(adj) != 15 {
		t.Fatalf("n=%d edges=%d", len(adj), Edges(adj))
	}
	if ok, why := Girth5(adj); !ok {
		t.Fatal(why)
	}
	// Adding any missing edge creates a triangle or a 4-cycle.
	for i := 0; i < 10; i++ {
		for j := i + 1; j < 10; j++ {
			if adj[i][j] {
				continue
			}
			adj[i][j], adj[j][i] = true, true
			if ok, _ := Girth5(adj); ok {
				t.Fatalf("adding %d-%d kept girth 5", i, j)
			}
			adj[i][j], adj[j][i] = false, false
		}
	}
}

func TestParseRejects(t *testing.T) {
	for _, s := range []string{"", "IheA@GUA", "IheA@GUAoo", "I\x01eA@GUAo"} {
		if _, err := ParseGraph6(s); err == nil {
			t.Errorf("ParseGraph6(%q) should fail", s)
		}
	}
}
