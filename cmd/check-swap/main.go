// Command check-swap checks the certificates written by `wb swap -certs`.
//
// The claim being checked: no m rows of the array can be replaced by m-1
// new rows and keep it a covering array. For each m-subset S of rows, the
// certificate lists m requirements (t columns and t values each) such that
//
//   - no row outside S shows the requirement, so after deleting S the new
//     rows must cover it; and
//   - every two of them put different values in some shared column, so no
//     single row covers two of them.
//
// Then m-1 new rows cannot cover all m. The checker confirms both facts
// for every record, and that the records cover every m-subset exactly
// once. Like verify-ca it imports only the Go standard library.
//
//	check-swap ARRAY CERTS
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math/bits"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: check-swap ARRAY CERTS")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2], os.Stdout); err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
}

func run(arrayPath, certPath string, out io.Writer) error {
	raw, err := os.ReadFile(arrayPath)
	if err != nil {
		return err
	}
	rows, err := parse(string(raw))
	if err != nil {
		return err
	}
	f, err := os.Open(certPath)
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<20)
	header, err := br.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading header: %v", err)
	}
	h := map[string]string{}
	fields := strings.Fields(header)
	// "mathbench" is the project's old name; both mark the same v1 format.
	if len(fields) < 3 || (fields[0] != "qed" && fields[0] != "mathbench") || fields[1] != "swap-certs" || fields[2] != "v1" {
		return fmt.Errorf("not a v1 swap certificate file")
	}
	for _, kv := range fields[3:] {
		k, v, _ := strings.Cut(kv, "=")
		h[k] = v
	}
	num := func(key string) int {
		x, err := strconv.Atoi(h[key])
		if err != nil {
			return -1
		}
		return x
	}
	n, t, k, v, m := num("n"), num("t"), num("k"), num("v"), num("m")
	if want := fmt.Sprintf("%x", sha256.Sum256(raw)); h["sha256"] != want {
		return fmt.Errorf("certificates are for sha256 %s, array is %s", h["sha256"], want)
	}
	if n != len(rows) || k != len(rows[0]) || n > 64 || t < 1 || t > k || v < 2 || m < 1 || m > n {
		return fmt.Errorf("header %q does not match a %d by %d array", strings.TrimSpace(header), len(rows), len(rows[0]))
	}
	for _, r := range rows {
		for _, x := range r {
			if x >= v {
				return fmt.Errorf("array has symbol %d, header says v=%d", x, v)
			}
		}
	}
	total := choose(n, m)
	seen := make([]uint64, (total+63)/64)
	rec := make([]byte, 8+m*2*t)
	count := 0
	cols := make([][]int, m)
	vals := make([][]int, m)
	for i := range cols {
		cols[i] = make([]int, t)
		vals[i] = make([]int, t)
	}
	for {
		_, err := io.ReadFull(br, rec)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("record %d: truncated", count)
		}
		S := binary.LittleEndian.Uint64(rec)
		if bits.OnesCount64(S) != m || (n < 64 && S>>uint(n) != 0) {
			return fmt.Errorf("record %d: row set %x is not an %d-subset of %d rows", count, S, m, n)
		}
		r := rank(S, n, m)
		if seen[r/64]>>(r%64)&1 == 1 {
			return fmt.Errorf("record %d: subset %x appears twice", count, S)
		}
		seen[r/64] |= 1 << (r % 64)
		p := 8
		for i := 0; i < m; i++ {
			for j := 0; j < t; j++ {
				cols[i][j] = int(rec[p+j])
				vals[i][j] = int(rec[p+t+j])
			}
			p += 2 * t
			for j := 0; j < t; j++ {
				if cols[i][j] >= k || (j > 0 && cols[i][j] <= cols[i][j-1]) || vals[i][j] >= v {
					return fmt.Errorf("record %d: requirement %d is malformed", count, i)
				}
			}
			// No row outside S may show this requirement.
			for ri, row := range rows {
				if S>>uint(ri)&1 == 1 {
					continue
				}
				shows := true
				for j := 0; j < t; j++ {
					if row[cols[i][j]] != vals[i][j] {
						shows = false
						break
					}
				}
				if shows {
					return fmt.Errorf("record %d: row %d outside the subset covers requirement %d", count, ri, i)
				}
			}
		}
		// Every two requirements must clash in a shared column.
		for i := 0; i < m; i++ {
			for i2 := i + 1; i2 < m; i2++ {
				if !clash(cols[i], vals[i], cols[i2], vals[i2]) {
					return fmt.Errorf("record %d: requirements %d and %d could share a row", count, i, i2)
				}
			}
		}
		count++
	}
	if count != total {
		return fmt.Errorf("%d records, but there are %d subsets of %d rows out of %d", count, total, m, n)
	}
	fmt.Fprintf(out, "VERIFIED: all %d subsets of %d rows have a valid clique certificate, so no %d rows can be replaced by %d (array sha256=%s)\n",
		total, m, m, m-1, h["sha256"])
	return nil
}

func clash(c1, v1, c2, v2 []int) bool {
	for i, c := range c1 {
		for j, d := range c2 {
			if c == d && v1[i] != v2[j] {
				return true
			}
		}
	}
	return false
}

func choose(n, m int) int {
	r := 1
	for i := 0; i < m; i++ {
		r = r * (n - i) / (i + 1)
	}
	return r
}

// rank maps an m-subset to a number in [0, choose(n, m)) in the
// combinatorial number system: sum of choose(c_i, i+1) over its sorted
// elements c_0 < c_1 < ...
func rank(S uint64, n, m int) int {
	r, i := 0, 0
	for c := 0; c < n; c++ {
		if S>>uint(c)&1 == 1 {
			i++
			r += choose(c, i)
		}
	}
	return r
}

func parse(s string) ([][]int, error) {
	var rows [][]int
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		row := make([]int, len(f))
		for i, x := range f {
			n, err := strconv.Atoi(x)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("bad symbol %q", x)
			}
			row[i] = n
		}
		if len(rows) > 0 && len(row) != len(rows[0]) {
			return nil, fmt.Errorf("ragged rows")
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no rows")
	}
	return rows, nil
}
