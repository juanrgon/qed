// Command wb is the mathbench workbench.
//
//	wb verify FILE                 check a covering array with the plain verifier
//	wb swap FILE -m 6              can any m rows be replaced by m-1?
//	wb search -n 41 -model split   structured annealing search, logged to a ledger
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juanrgon/mathbench/internal/ca"
	"github.com/juanrgon/mathbench/internal/classify"
	"github.com/juanrgon/mathbench/internal/engine"
	"github.com/juanrgon/mathbench/internal/lightslice"
	"github.com/juanrgon/mathbench/internal/run"
	"github.com/juanrgon/mathbench/internal/swap"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "verify":
		cmdVerify(os.Args[2:])
	case "swap":
		cmdSwap(os.Args[2:])
	case "search":
		cmdSearch(os.Args[2:])
	case "lightslice":
		cmdLightslice(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wb verify|swap|search [flags]  (wb <cmd> -h for flags)")
	os.Exit(2)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func cmdVerify(args []string) {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	t := fs.Int("t", 3, "strength")
	v := fs.Int("v", 3, "symbols")
	fs.Parse(args)
	if fs.NArg() != 1 {
		die(fmt.Errorf("need one file"))
	}
	a, err := ca.ReadFile(fs.Arg(0), *v)
	if err != nil {
		die(err)
	}
	rep := ca.Verify(a, *t, *v)
	if rep.OK() {
		fmt.Printf("VERIFIED: CA(N=%d; t=%d, k=%d, v=%d)\n", a.N, *t, a.K, *v)
		return
	}
	fmt.Printf("NOT A COVERING ARRAY: %d requirements missing\n", rep.Missing)
	for _, g := range rep.Examples {
		fmt.Printf("  cols %v never show %v\n", g.Cols, g.Vals)
	}
	os.Exit(1)
}

func cmdSwap(args []string) {
	fs := flag.NewFlagSet("swap", flag.ExitOnError)
	t := fs.Int("t", 3, "strength")
	v := fs.Int("v", 3, "symbols")
	m := fs.Int("m", 6, "rows to remove (try to replace with m-1)")
	workers := fs.Int("workers", 8, "worker goroutines (8 = the performance cores)")
	nodeCap := fs.Int64("cap", 1<<22, "node cap per clique or coloring search")
	out := fs.String("out", "runs/swap-witnesses", "where to save any verified smaller array")
	certPath := fs.String("certs", "", "write a clique certificate for every subset to this file (check it with check-swap)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		die(fmt.Errorf("need one file"))
	}
	a, err := ca.ReadFile(fs.Arg(0), *v)
	if err != nil {
		die(err)
	}
	if !ca.Verify(a, *t, *v).OK() {
		die(fmt.Errorf("input is not a covering array"))
	}
	var emit func(swap.Cert)
	var flush func() error
	if *certPath != "" {
		raw, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die(err)
		}
		f, err := os.Create(*certPath)
		if err != nil {
			die(err)
		}
		bw := bufio.NewWriterSize(f, 1<<20)
		fmt.Fprintf(bw, "mathbench swap-certs v1 n=%d t=%d k=%d v=%d m=%d sha256=%x\n", a.N, *t, a.K, *v, *m, sha256.Sum256(raw))
		var mu sync.Mutex
		rec := make([]byte, 8)
		emit = func(c swap.Cert) {
			mu.Lock()
			defer mu.Unlock()
			binary.LittleEndian.PutUint64(rec, c.Rows)
			bw.Write(rec)
			for i := range c.Cols {
				for _, col := range c.Cols[i] {
					bw.WriteByte(byte(col))
				}
				bw.Write(c.Vals[i])
			}
		}
		flush = func() error {
			if err := bw.Flush(); err != nil {
				return err
			}
			return f.Close()
		}
	}
	t0 := time.Now()
	s := swap.CheckCerts(a, *t, *v, *m, *workers, *nodeCap, emit)
	if flush != nil {
		if err := flush(); err != nil {
			die(err)
		}
		fmt.Printf("  certificates written to %s (%d cliques)\n", *certPath, s.ByClique)
	}
	fmt.Printf("%d -> %d on %s (N=%d): %d subsets in %s\n", *m, *m-1, filepath.Base(fs.Arg(0)), a.N, s.Subsets, time.Since(t0).Round(time.Millisecond))
	fmt.Printf("  impossible: %d (clique proof %d, exhaustive coloring %d)\n", s.Impossible(), s.ByClique, s.ByColoring)
	fmt.Printf("  undetermined (cap hit): %d\n", s.Undetermined)
	fmt.Printf("  verified witnesses (N=%d arrays): %d\n", a.N-1, len(s.Witnesses))
	fmt.Printf("  largest U: %d requirements\n", s.MaxU)
	if s.BadWitnesses > 0 {
		fmt.Printf("  BUG: %d colorings failed verification\n", s.BadWitnesses)
	}
	if len(s.Witnesses) > 0 {
		os.MkdirAll(*out, 0o755)
		for i, w := range s.Witnesses {
			p := filepath.Join(*out, fmt.Sprintf("CA_%d_%d_%d_%d.%d.txt", w.N, *t, w.K, *v, i))
			if err := w.WriteFile(p); err != nil {
				die(err)
			}
			fmt.Println("  saved", p)
		}
	}
}

func parseInts(s string) ([]int, error) {
	var out []int
	for _, f := range strings.Split(s, ",") {
		x, err := strconv.Atoi(strings.TrimSpace(f))
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, nil
}

func parseSeeds(s string) ([]uint64, error) {
	lo, hi, ok := strings.Cut(s, ":")
	if !ok {
		x, err := strconv.ParseUint(s, 10, 64)
		return []uint64{x}, err
	}
	a, err := strconv.ParseUint(lo, 10, 64)
	if err != nil {
		return nil, err
	}
	b, err := strconv.ParseUint(hi, 10, 64)
	if err != nil {
		return nil, err
	}
	var out []uint64
	for x := a; x < b; x++ {
		out = append(out, x)
	}
	return out, nil
}

func cmdSearch(args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	t := fs.Int("t", 3, "strength")
	k := fs.Int("k", 8, "columns")
	v := fs.Int("v", 3, "symbols")
	n := fs.Int("n", 42, "rows (free model)")
	model := fs.String("model", "free", "free | split | shift | orbit")
	group := fs.String("group", "c7z3", "group for -model orbit: z3 | c7 | ck | c7z3 | ckz3")
	split := fs.String("split", "13,14,14", "column-0 symbol counts for -model split")
	reps := fs.Int("reps", 14, "orbits for -model shift or orbit")
	free := fs.Int("free", 0, "extra free rows for -model shift or orbit")
	seeds := fs.String("seeds", "0:8", "seed or lo:hi range")
	p := engine.DefaultParams()
	fs.Int64Var(&p.Moves, "moves", p.Moves, "move budget per seed")
	fs.IntVar(&p.Sample, "sample", p.Sample, "rows tried per move")
	fs.Float64Var(&p.T0, "t0", p.T0, "start temperature")
	fs.Float64Var(&p.T1, "t1", p.T1, "end temperature")
	fs.Float64Var(&p.Noise, "noise", p.Noise, "random-move probability")
	workers := fs.Int("workers", 8, "parallel seeds (one per performance core)")
	ledgerPath := fs.String("ledger", "runs/ledger.jsonl", "JSONL ledger")
	outDir := fs.String("out", "runs/solutions", "where verified solutions go")
	cpuprof := fs.String("cpuprofile", "", "write a CPU profile")
	fs.Parse(args)

	if *cpuprof != "" {
		f, err := os.Create(*cpuprof)
		if err != nil {
			die(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	mk := func() *engine.Model {
		switch *model {
		case "free":
			return engine.Free(*n, *k, *v)
		case "split":
			sp, err := parseInts(*split)
			if err != nil || len(sp) != *v {
				die(fmt.Errorf("bad -split %q", *split))
			}
			return engine.SplitColumn0(*k, *v, sp)
		case "shift":
			return engine.ShiftOrbits(*reps, *free, *k, *v)
		case "orbit":
			g, err := engine.Group(*group, *k, *v)
			if err != nil {
				die(err)
			}
			return engine.Orbits(*reps, *free, *k, *v, *group, g)
		}
		die(fmt.Errorf("unknown model %q", *model))
		return nil
	}
	m0 := mk()
	sl, err := parseSeeds(*seeds)
	if err != nil {
		die(err)
	}
	os.MkdirAll(filepath.Dir(*ledgerPath), 0o755)
	l, err := run.OpenLedger(*ledgerPath)
	if err != nil {
		die(err)
	}
	defer l.Close()
	ix := ca.NewIndex(*t, *k, *v)
	job := fmt.Sprintf("CA(%d;%d,%d,%d) %s moves=%d sample=%d T=%g..%g noise=%g",
		m0.N, *t, *k, *v, m0.Name, p.Moves, p.Sample, p.T0, p.T1, p.Noise)
	fmt.Println("job:", job)
	t0 := time.Now()
	recs := run.Seeds(l, job, sl, *workers, func(seed uint64) run.Record {
		pp := p
		pp.Seed = seed
		out := engine.Anneal(mk(), ix, pp)
		r := run.Record{Solved: out.Solved, Best: out.Best, Moves: out.Moves}
		if out.Solved {
			// The engine's claim only counts once the plain verifier agrees.
			r.Verified = ca.Verify(out.Array, *t, *v).OK()
			if r.Verified {
				os.MkdirAll(*outDir, 0o755)
				r.Artifact = filepath.Join(*outDir, fmt.Sprintf("CA_%d_%d_%d_%d.seed%d.txt", out.Array.N, *t, *k, *v, seed))
				if err := out.Array.WriteFile(r.Artifact); err != nil {
					die(err)
				}
			}
		}
		tag := "best=" + strconv.Itoa(out.Best)
		if r.Verified {
			tag = "SOLVED+VERIFIED " + r.Artifact
		}
		fmt.Printf("  seed %d: %s\n", seed, tag)
		return r
	})
	summarize(recs, time.Since(t0))
}

// cmdLightslice runs the full proof that no CA(38; 3, 8, 3) exists.
//
// Step 1 classifies small strength-2 arrays from scratch: no
// CA(11; 2, 7, 3) exists, so every slice has at least 12 rows, and
// CA(12; 2, 7, 3) has exactly one isomorphism class, so column 0's light
// slice can be pinned. Step 2 pins it and looks for contradictions. With
// a FILE argument, step 2 pins that copy instead, after checking that it
// is isomorphic to the class found in step 1.
func cmdLightslice(args []string) {
	fs := flag.NewFlagSet("lightslice", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() > 1 {
		die(fmt.Errorf("at most one CA(12; 2, 7, 3) file"))
	}
	t0 := time.Now()
	l11 := classify.Levels(11, 7, 3)
	levels := classify.Levels(12, 7, 3)
	fmt.Printf("step 1 (%s): CA(11;2,7,3) classes: %d; CA(12;2,7,3) classes: %d\n",
		time.Since(t0).Round(time.Millisecond), len(l11[6]), len(levels[6]))
	if len(l11[6]) != 0 || len(levels[6]) != 1 {
		die(fmt.Errorf("classification does not support the argument"))
	}
	rep := levels[6][0]
	if !classify.IsCA(rep, 3) {
		die(fmt.Errorf("class representative is not a CA(12; 2, 7, 3)"))
	}
	r := ca.New(0, 7)
	for i := 0; i < rep.N(); i++ {
		row := make([]uint8, 7)
		for c := range row {
			row[c] = rep[c][i]
		}
		r.AppendRow(row)
	}
	if fs.NArg() == 1 {
		var err error
		r, err = ca.ReadFile(fs.Arg(0), 3)
		if err != nil {
			die(err)
		}
		if r.N != 12 || r.K != 7 || !ca.Verify(r, 2, 3).OK() {
			die(fmt.Errorf("input is not a CA(12; 2, 7, 3)"))
		}
		rows := make([][]uint8, r.N)
		for i := range rows {
			rows[i] = r.Row(i)
		}
		x := classify.FromRows(rows)
		m, ok := classify.FindIso(x, rep, 3)
		if !ok || !classify.SameUpToRows(classify.Apply(x, m), rep) {
			die(fmt.Errorf("input is not isomorphic to the class found in step 1"))
		}
		fmt.Println("input file is isomorphic to it (map checked)")
	}
	if !ca.Verify(r, 2, 3).OK() {
		die(fmt.Errorf("pinned slice does not verify"))
	}
	t0 = time.Now()
	t := lightslice.Build(lightslice.Pinned(r), 8, 12)
	fmt.Printf("candidate light slices per column (symbol 0,1,2), found in %s:\n", time.Since(t0).Round(time.Millisecond))
	for Y := 1; Y < t.K; Y++ {
		fmt.Printf("  col %d: %d %d %d\n", Y, len(t.Cands[Y][0]), len(t.Cands[Y][1]), len(t.Cands[Y][2]))
	}
	bad := 0
	fmt.Println("compatible pairs per column pair (rows v = symbol in Y, cols w = symbol in Z):")
	for Y := 1; Y < t.K; Y++ {
		for Z := Y + 1; Z < t.K; Z++ {
			m := t.Matrix(Y, Z)
			total := 0
			for _, row := range m {
				for _, x := range row {
					total += x
				}
			}
			tag := ""
			if total == 0 {
				tag = "  <- no compatible pair: obstruction"
				bad++
			}
			fmt.Printf("  (%d,%d): %v%s\n", Y, Z, m, tag)
		}
	}
	fmt.Printf("%d of 21 column pairs are full obstructions\n", bad)
	if bad > 0 {
		fmt.Println("RESULT: no CA(38; 3, 8, 3) exists, so CAN(3, 8, 3) >= 39")
	}
}

func summarize(recs []run.Record, wall time.Duration) {
	if len(recs) == 0 {
		fmt.Println("nothing to run: every seed is already in the ledger")
		return
	}
	solved := 0
	var bests []int
	var mps []float64
	for _, r := range recs {
		if r.Verified {
			solved++
		}
		bests = append(bests, r.Best)
		mps = append(mps, r.MovesPerS)
	}
	slices.Sort(bests)
	slices.Sort(mps)
	fmt.Printf("solved+verified %d/%d | best missing min %d median %d | moves/s per core median %.2fM | wall %s\n",
		solved, len(recs), bests[0], bests[len(bests)/2], mps[len(mps)/2]/1e6, wall.Round(time.Second))
}
