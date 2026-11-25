// Command verify-ca is the standalone judge for covering arrays.
//
// It imports only the Go standard library, never this repo's packages,
// so a bug in the search code cannot also hide in the judge. It reads an
// array from disk (one row per line, symbols separated by spaces), checks
// every t-subset of columns for every t-tuple of symbols the slow way,
// and prints the file's sha256 with the verdict.
//
//	verify-ca -t 3 -v 3 FILE
//
// Exit status 0 means the file is a covering array with those parameters.
package main

import (
	"bufio"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func main() {
	t := flag.Int("t", 3, "strength")
	v := flag.Int("v", 3, "number of symbols")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: verify-ca [-t T] [-v V] FILE")
		os.Exit(2)
	}
	raw, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fail(err.Error())
	}
	rows, err := parse(string(raw), *v)
	if err != nil {
		fail(err.Error())
	}
	n, k := len(rows), len(rows[0])
	sum := sha256.Sum256(raw)
	if *t < 1 || *t > k {
		fail(fmt.Sprintf("strength %d does not fit %d columns", *t, k))
	}
	missing, example := check(rows, *t, *v)
	if missing > 0 {
		fmt.Printf("FAIL: %d missing t-tuples, e.g. %s; N=%d t=%d k=%d v=%d sha256=%x\n",
			missing, example, n, *t, k, *v, sum)
		os.Exit(1)
	}
	fmt.Printf("VERIFIED: CA(%d; %d, %d, %d) sha256=%x\n", n, *t, k, *v, sum)
}

func fail(msg string) {
	fmt.Println("FAIL:", msg)
	os.Exit(1)
}

func parse(s string, v int) ([][]int, error) {
	var rows [][]int
	sc := bufio.NewScanner(strings.NewReader(s))
	line := 0
	for sc.Scan() {
		line++
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		row := make([]int, len(f))
		for i, x := range f {
			n, err := strconv.Atoi(x)
			if err != nil || n < 0 || n >= v {
				return nil, fmt.Errorf("line %d: bad symbol %q", line, x)
			}
			row[i] = n
		}
		if len(rows) > 0 && len(row) != len(rows[0]) {
			return nil, fmt.Errorf("line %d: %d columns, want %d", line, len(row), len(rows[0]))
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no rows")
	}
	return rows, nil
}

// check walks every t-subset of columns in plain lexicographic order and
// every t-tuple of symbols, and counts the tuples no row shows.
func check(rows [][]int, t, v int) (int, string) {
	k := len(rows[0])
	cols := make([]int, t)
	for i := range cols {
		cols[i] = i
	}
	tuples := 1
	for i := 0; i < t; i++ {
		tuples *= v
	}
	missing := 0
	example := ""
	for {
		seen := make([]bool, tuples)
		for _, r := range rows {
			code := 0
			for _, c := range cols {
				code = code*v + r[c]
			}
			seen[code] = true
		}
		for code, ok := range seen {
			if ok {
				continue
			}
			missing++
			if example == "" {
				vals := make([]int, t)
				for i, x := t-1, code; i >= 0; i-- {
					vals[i] = x % v
					x /= v
				}
				example = fmt.Sprintf("columns %v never show %v", cols, vals)
			}
		}
		// next t-subset
		i := t - 1
		for i >= 0 && cols[i] == k-t+i {
			i--
		}
		if i < 0 {
			return missing, example
		}
		cols[i]++
		for j := i + 1; j < t; j++ {
			cols[j] = cols[j-1] + 1
		}
	}
}
