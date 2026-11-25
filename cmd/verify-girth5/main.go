// Command verify-girth5 is the standalone judge for graphs of girth at
// least 5 (no triangle and no 4-cycle).
//
// It imports only the Go standard library. It reads graph6 lines (one
// graph per line) and, for each, prints the number of vertices, the number
// of edges, and whether the graph has girth at least 5. The check is the
// plain one: no two distinct vertices may have two common neighbors (a
// 4-cycle), and no adjacent pair may have one (a triangle).
//
//	verify-girth5 [-min-edges E] FILE
//
// Exit status 0 means every graph passed (and has at least E edges).
package main

import (
	"bufio"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"strings"
)

func main() {
	minEdges := flag.Int("min-edges", 0, "fail unless every graph has at least this many edges")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: verify-girth5 [-min-edges E] FILE")
		os.Exit(2)
	}
	raw, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	bad, count := 0, 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		count++
		adj, err := ParseGraph6(line)
		if err != nil {
			fmt.Printf("graph %d: FAIL: %v\n", count, err)
			bad++
			continue
		}
		n, e := len(adj), Edges(adj)
		ok, why := Girth5(adj)
		sum := sha256.Sum256([]byte(line))
		switch {
		case !ok:
			fmt.Printf("graph %d: FAIL: n=%d edges=%d: %s sha256=%x\n", count, n, e, why, sum)
			bad++
		case e < *minEdges:
			fmt.Printf("graph %d: FAIL: n=%d edges=%d < %d sha256=%x\n", count, n, e, *minEdges, sum)
			bad++
		default:
			fmt.Printf("graph %d: VERIFIED: n=%d edges=%d girth>=5 sha256=%x\n", count, n, e, sum)
		}
	}
	if count == 0 {
		fmt.Println("FAIL: no graphs")
		os.Exit(1)
	}
	if bad > 0 {
		os.Exit(1)
	}
}

// ParseGraph6 decodes one graph6 string into an adjacency matrix.
func ParseGraph6(s string) ([][]bool, error) {
	b := []byte(s)
	for _, c := range b {
		if c < 63 || c > 126 {
			return nil, fmt.Errorf("byte %d outside graph6 range", c)
		}
	}
	var n, pos int
	switch {
	case len(b) >= 1 && b[0] != 126:
		n, pos = int(b[0])-63, 1
	case len(b) >= 4 && b[1] != 126:
		n = (int(b[1])-63)<<12 | (int(b[2])-63)<<6 | (int(b[3]) - 63)
		pos = 4
	default:
		return nil, fmt.Errorf("unsupported graph6 size header")
	}
	bitsNeeded := n * (n - 1) / 2
	if len(b)-pos != (bitsNeeded+5)/6 {
		return nil, fmt.Errorf("length %d does not match n=%d", len(b), n)
	}
	adj := make([][]bool, n)
	for i := range adj {
		adj[i] = make([]bool, n)
	}
	k := 0
	for j := 1; j < n; j++ {
		for i := 0; i < j; i++ {
			c := int(b[pos+k/6]) - 63
			if c>>(5-k%6)&1 == 1 {
				adj[i][j], adj[j][i] = true, true
			}
			k++
		}
	}
	// Padding bits must be zero.
	for ; k%6 != 0; k++ {
		if (int(b[pos+k/6])-63)>>(5-k%6)&1 == 1 {
			return nil, fmt.Errorf("nonzero padding bits")
		}
	}
	return adj, nil
}

// Edges counts the edges.
func Edges(adj [][]bool) int {
	e := 0
	for i := range adj {
		for j := i + 1; j < len(adj); j++ {
			if adj[i][j] {
				e++
			}
		}
	}
	return e
}

// Girth5 reports whether the graph has no triangle and no 4-cycle.
func Girth5(adj [][]bool) (bool, string) {
	n := len(adj)
	for a := 0; a < n; a++ {
		for b := a + 1; b < n; b++ {
			common := 0
			for c := 0; c < n; c++ {
				if c != a && c != b && adj[a][c] && adj[b][c] {
					common++
				}
			}
			if common >= 2 {
				return false, fmt.Sprintf("vertices %d and %d have %d common neighbors (4-cycle)", a, b, common)
			}
			if common == 1 && adj[a][b] {
				return false, fmt.Sprintf("edge %d-%d has a common neighbor (triangle)", a, b)
			}
		}
	}
	return true, ""
}
