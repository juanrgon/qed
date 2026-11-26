// Command wb is the qed workbench.
//
//	wb verify FILE                 check a covering array with the plain verifier
//	wb swap FILE -m 6              can any m rows be replaced by m-1?
//	wb search -n 41 -model split   structured annealing search, logged to a ledger
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juanrgon/qed/internal/ca"
	"github.com/juanrgon/qed/internal/classify"
	"github.com/juanrgon/qed/internal/core"
	"github.com/juanrgon/qed/internal/domains/girth5"
	"github.com/juanrgon/qed/internal/domains/matmul"
	"github.com/juanrgon/qed/internal/engine"
	"github.com/juanrgon/qed/internal/lightslice"
	"github.com/juanrgon/qed/internal/run"
	"github.com/juanrgon/qed/internal/sat"
	"github.com/juanrgon/qed/internal/swap"
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
	case "claims":
		cmdClaims(os.Args[2:])
	case "girth5-reroute":
		cmdGirth5Reroute(os.Args[2:])
	case "girth5-lns":
		cmdGirth5LNS(os.Args[2:])
	case "matmul":
		cmdMatmul(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wb verify|swap|search|lightslice|claims|girth5-reroute [flags]  (wb <cmd> -h for flags)")
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
	// With -certs -, certificates go to stdout and messages to stderr, so
	// they can be piped straight into check-swap.
	var msg io.Writer = os.Stdout
	if *certPath == "-" {
		msg = os.Stderr
	}
	var emit func(swap.Cert)
	var flush func() error
	if *certPath != "" {
		raw, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die(err)
		}
		f, err := createOrStdout(*certPath)
		if err != nil {
			die(err)
		}
		bw := bufio.NewWriterSize(f, 1<<20)
		fmt.Fprintf(bw, "qed swap-certs v1 n=%d t=%d k=%d v=%d m=%d sha256=%x\n", a.N, *t, a.K, *v, *m, sha256.Sum256(raw))
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
		fmt.Fprintf(msg, "  certificates written to %s (%d cliques)\n", *certPath, s.ByClique)
	}
	fmt.Fprintf(msg, "%d -> %d on %s (N=%d): %d subsets in %s\n", *m, *m-1, filepath.Base(fs.Arg(0)), a.N, s.Subsets, time.Since(t0).Round(time.Millisecond))
	fmt.Fprintf(msg, "  impossible: %d (clique proof %d, exhaustive coloring %d)\n", s.Impossible(), s.ByClique, s.ByColoring)
	fmt.Fprintf(msg, "  undetermined (cap hit): %d\n", s.Undetermined)
	fmt.Fprintf(msg, "  verified witnesses (N=%d arrays): %d\n", a.N-1, len(s.Witnesses))
	fmt.Fprintf(msg, "  largest U: %d requirements\n", s.MaxU)
	if s.BadWitnesses > 0 {
		fmt.Fprintf(msg, "  BUG: %d colorings failed verification\n", s.BadWitnesses)
	}
	if len(s.Witnesses) > 0 {
		os.MkdirAll(*out, 0o755)
		for i, w := range s.Witnesses {
			p := filepath.Join(*out, fmt.Sprintf("CA_%d_%d_%d_%d.%d.txt", w.N, *t, w.K, *v, i))
			if err := w.WriteFile(p); err != nil {
				die(err)
			}
			fmt.Fprintln(msg, "  saved", p)
		}
	}
}

// cmdClaims prints every claim in claims/ with its computed status. With
// -run it also re-checks each claim: artifact hashes, then the recheck
// commands (and the slow ones with -slow). It exits 1 if any check fails.
func cmdClaims(args []string) {
	fs := flag.NewFlagSet("claims", flag.ExitOnError)
	dir := fs.String("dir", "claims", "claims folder")
	runChecks := fs.Bool("run", false, "re-run every claim's checks")
	slow := fs.Bool("slow", false, "with -run, also run the slow checks")
	fs.Parse(args)
	all, err := core.Load(*dir)
	if err != nil {
		die(err)
	}
	var ids []string
	for id := range all {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	failed := 0
	for _, id := range ids {
		c := all[id]
		st, err := core.Compute(all, id)
		if err != nil {
			die(err)
		}
		fmt.Printf("%s\n  %s\n  status: %s\n", id, c.Statement, st)
		if c.Literature != nil {
			fmt.Printf("  literature (checked %s): %s\n", c.Literature.Checked, c.Literature.Best)
		}
		if !*runChecks {
			continue
		}
		if c.Cert.Artifact != "" {
			raw, err := os.ReadFile(c.Cert.Artifact)
			got := fmt.Sprintf("%x", sha256.Sum256(raw))
			if err != nil || got != c.Cert.SHA256 {
				fmt.Printf("  FAIL artifact %s: sha256 %s, claim says %s\n", c.Cert.Artifact, got, c.Cert.SHA256)
				failed++
			}
		}
		cmds := c.Cert.Recheck
		if *slow {
			cmds = append(slices.Clone(cmds), c.Cert.SlowRecheck...)
		}
		for _, cmd := range cmds {
			t0 := time.Now()
			out, err := exec.Command("bash", "-c", "set -o pipefail; "+cmd).CombinedOutput()
			if err != nil {
				fmt.Printf("  FAIL (%s): %s\n%s\n", time.Since(t0).Round(time.Second), cmd, out)
				failed++
				continue
			}
			fmt.Printf("  ok (%s): %s\n", time.Since(t0).Round(time.Second), cmd)
		}
	}
	if failed > 0 {
		fmt.Printf("%d checks failed\n", failed)
		os.Exit(1)
	}
}

// cmdGirth5Reroute tries every k-subset R of vertices of a graph: delete
// the edges touching R and ask CaDiCaL whether edges touching R can be
// added back to reach -target edges with girth at least 5. A SAT answer
// is saved and must pass cmd/verify-girth5. An UNSAT answer must come with
// an LRAT proof that cmd/check-lrat accepts; otherwise it counts as a
// failure, not as a proof.
func cmdGirth5Reroute(args []string) {
	fs := flag.NewFlagSet("girth5-reroute", flag.ExitOnError)
	k := fs.Int("k", 1, "vertices to reroute at once")
	target := fs.Int("target", 0, "edges wanted (default: current + 1)")
	line := fs.Int("line", 1, "which graph6 line of FILE to use (1-based)")
	workers := fs.Int("workers", 8, "parallel solver jobs")
	outDir := fs.String("out", "runs/girth5", "where proofs, found graphs and the summary go")
	noCake := fs.Bool("no-cake", false, "skip the cake_lpr check even if it is installed")
	fs.Parse(args)
	if fs.NArg() != 1 {
		die(fmt.Errorf("need a graph6 file"))
	}
	raw, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		die(err)
	}
	lines := strings.Fields(string(raw))
	if *line < 1 || *line > len(lines) {
		die(fmt.Errorf("file has %d graphs", len(lines)))
	}
	g6 := lines[*line-1]
	g, err := girth5.ParseGraph6(g6)
	if err != nil {
		die(err)
	}
	if *target == 0 {
		*target = g.Edges() + 1
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(g6)))
	dir := filepath.Join(*outDir, sum[:12], fmt.Sprintf("k%d-t%d", *k, *target))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		die(err)
	}
	checker := filepath.Join(*outDir, "bin", "check-lrat")
	if out, err := exec.Command("go", "build", "-o", checker, "./cmd/check-lrat").CombinedOutput(); err != nil {
		die(fmt.Errorf("building check-lrat: %v\n%s", err, out))
	}
	// When cake_lpr (a formally verified checker) is installed, every
	// proof must pass both checkers.
	cake, _ := sat.CakeLPR()
	if *noCake {
		cake = ""
	}
	n := len(g)
	fmt.Printf("graph sha256=%s n=%d edges=%d; rerouting every %d-subset toward %d edges\n", sum, n, g.Edges(), *k, *target)
	if cake != "" {
		fmt.Println("proofs are checked by cmd/check-lrat and by cake_lpr at", cake)
	}

	var subsets [][]int
	var rec func(start int, cur []int)
	rec = func(start int, cur []int) {
		if len(cur) == *k {
			subsets = append(subsets, slices.Clone(cur))
			return
		}
		for v := start; v < n; v++ {
			rec(v+1, append(cur, v))
		}
	}
	rec(0, nil)

	type result struct {
		R      []int
		Status string // unsat-checked | sat | error
		Detail string
	}
	jobs := make(chan []int)
	results := make(chan result)
	var wg sync.WaitGroup
	for w := 0; w < *workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for R := range jobs {
				p := girth5.NewReroute(g, R, *target)
				name := strings.Trim(strings.ReplaceAll(fmt.Sprint(R), " ", "-"), "[]")
				cnf := filepath.Join(dir, "r"+name+".cnf")
				proof := filepath.Join(dir, "r"+name+".lrat")
				res, err := sat.Solve(p.CNF, cnf, proof)
				switch {
				case err != nil:
					results <- result{R, "error", err.Error()}
				case res.Sat:
					h := p.Decode(res.Model)
					path := filepath.Join(dir, "found-r"+name+".g6")
					os.WriteFile(path, []byte(h.Graph6()+"\n"), 0o644)
					results <- result{R, "sat", path}
				default:
					out, err := exec.Command(checker, cnf, proof).CombinedOutput()
					if err != nil {
						results <- result{R, "error", "proof rejected: " + strings.TrimSpace(string(out))}
						continue
					}
					if cake != "" {
						if err := sat.CheckCake(cake, cnf, proof); err != nil {
							results <- result{R, "error", err.Error()}
							continue
						}
						out = append(out, []byte(" + cake_lpr: s VERIFIED UNSAT")...)
					}
					os.Remove(cnf)
					os.Remove(proof)
					results <- result{R, "unsat-checked", strings.TrimSpace(string(out))}
				}
			}
		}()
	}
	go func() {
		for _, R := range subsets {
			jobs <- R
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	t0 := time.Now()
	counts := map[string]int{}
	logf, err := os.Create(filepath.Join(dir, "summary.jsonl"))
	if err != nil {
		die(err)
	}
	enc := json.NewEncoder(logf)
	done := 0
	for r := range results {
		done++
		counts[r.Status]++
		enc.Encode(r)
		if r.Status != "unsat-checked" {
			fmt.Printf("  R=%v: %s %s\n", r.R, strings.ToUpper(r.Status), r.Detail)
		}
		if done%250 == 0 {
			fmt.Printf("  %d/%d done (%s)\n", done, len(subsets), time.Since(t0).Round(time.Second))
		}
	}
	logf.Close()
	fmt.Printf("%d subsets in %s: %d UNSAT with checked proofs, %d SAT, %d errors\n",
		len(subsets), time.Since(t0).Round(time.Second), counts["unsat-checked"], counts["sat"], counts["error"])
	if counts["sat"] > 0 {
		fmt.Println("SAT graphs were saved; check them with: go run ./cmd/verify-girth5 FILE")
	}
}

// cmdGirth5LNS runs the SAT-based large neighborhood search from the
// graph with the most edges in FILE (or from a greedy graph with -n), one
// independent run per seed. Every improvement is saved; a run that ends
// without one is logged as budget exhausted, which proves nothing.
func cmdGirth5LNS(args []string) {
	fs := flag.NewFlagSet("girth5-lns", flag.ExitOnError)
	n := fs.Int("n", 0, "start from a greedy graph on n vertices instead of FILE")
	k := fs.Int("k", 4, "vertices freed per step")
	iters := fs.Int("iters", 2000, "steps per seed")
	conflicts := fs.Int("conflicts", 2000, "solver conflict cap per query")
	ball := fs.Float64("ball", 0.7, "chance of a local ball instead of random vertices")
	stop := fs.Int("stop", 0, "stop a seed once it reaches this many edges")
	degrees := fs.Bool("degrees", true, "per-vertex degree targets (fast) instead of one global edge count")
	seeds := fs.String("seeds", "0:8", "seed or lo:hi range")
	workers := fs.Int("workers", 8, "parallel seeds")
	ledgerPath := fs.String("ledger", "runs/ledger.jsonl", "JSONL ledger")
	outDir := fs.String("out", "runs/girth5-lns", "where improved graphs go")
	fs.Parse(args)

	var start girth5.Graph
	src := fmt.Sprintf("greedy n=%d", *n)
	if *n == 0 {
		if fs.NArg() != 1 {
			die(fmt.Errorf("need a graph6 file or -n"))
		}
		raw, err := os.ReadFile(fs.Arg(0))
		if err != nil {
			die(err)
		}
		for _, l := range strings.Fields(string(raw)) {
			g, err := girth5.ParseGraph6(l)
			if err != nil {
				die(err)
			}
			if start == nil || g.Edges() > start.Edges() {
				start, src = g, fmt.Sprintf("%x", sha256.Sum256([]byte(l)))[:12]
			}
		}
	}
	sl, err := parseSeeds(*seeds)
	if err != nil {
		die(err)
	}
	os.MkdirAll(*outDir, 0o755)
	os.MkdirAll(filepath.Dir(*ledgerPath), 0o755)
	l, err := run.OpenLedger(*ledgerPath)
	if err != nil {
		die(err)
	}
	defer l.Close()
	job := fmt.Sprintf("girth5-lns from=%s k=%d iters=%d conflicts=%d ball=%g degrees=%v", src, *k, *iters, *conflicts, *ball, *degrees)
	fmt.Println("job:", job)
	t0 := time.Now()
	run.Seeds(l, job, sl, *workers, func(seed uint64) run.Record {
		g := start
		if g == nil {
			g = girth5.New(*n)
			girth5.Greedy(g, rand.New(rand.NewPCG(seed, 1)))
		}
		e0 := g.Edges()
		tmp, err := os.CreateTemp("", "girth5-*.cnf")
		if err != nil {
			die(err)
		}
		tmp.Close()
		defer os.Remove(tmp.Name())
		solve := func(f *sat.CNF) (sat.Result, error) { return sat.SolveLimit(f, tmp.Name(), *conflicts) }
		var artifact string
		best, st, err := girth5.LNS(g, girth5.LNSParams{Iters: *iters, K: *k, Seed: seed, Ball: *ball, Stop: *stop, Degrees: *degrees}, solve,
			func(h girth5.Graph) {
				p := filepath.Join(*outDir, fmt.Sprintf("n%d-e%d-seed%d.g6", len(h), h.Edges(), seed))
				os.WriteFile(p, []byte(h.Graph6()+"\n"), 0o644)
				artifact = p
				if start != nil {
					fmt.Printf("  seed %d: IMPROVED to %d edges (start %d): %s\n", seed, h.Edges(), e0, p)
				}
			})
		if err != nil {
			die(err)
		}
		fmt.Printf("  seed %d: %d -> %d edges; %d steps, %d up, %d sideways, %d stuck, %d capped\n",
			seed, e0, best.Edges(), st.Iters, st.Improved, st.Plateau, st.Stuck, st.Unknown)
		return run.Record{Solved: best.Edges() > e0 && start != nil, Best: best.Edges(), Moves: int64(st.Iters), Artifact: artifact}
	})
	fmt.Printf("wall %s\n", time.Since(t0).Round(time.Second))
}

// cmdMatmul runs flip graph walks for <n,m,p> matrix multiplication, one
// per seed, from the schoolbook scheme. Each new best scheme is saved and
// checked against the plain Brent equations; check saved files with
// cmd/verify-matmul.
func cmdMatmul(args []string) {
	fs := flag.NewFlagSet("matmul", flag.ExitOnError)
	n := fs.Int("n", 2, "rows of A")
	m := fs.Int("m", 2, "columns of A = rows of B")
	p := fs.Int("p", 2, "columns of B")
	steps := fs.Int64("steps", 10_000_000, "flip attempts per seed")
	maxAbs := fs.Int("max", 1, "largest coefficient allowed")
	stall := fs.Int64("stall", 50_000, "flips without progress before a plus move")
	restart := fs.Int64("restart", 500_000, "flips spent above the best before going back to it")
	target := fs.Int("target", 0, "stop a seed once it reaches this rank")
	seeds := fs.String("seeds", "0:8", "seed or lo:hi range")
	workers := fs.Int("workers", 8, "parallel seeds")
	ledgerPath := fs.String("ledger", "runs/ledger.jsonl", "JSONL ledger")
	outDir := fs.String("out", "runs/matmul", "where schemes go")
	pool := fs.Int("pool", 0, "use the level-by-level pool search, keeping this many schemes per level")
	walkLen := fs.Int64("walk", 2_000_000, "pool search: flips per walk")
	walks := fs.Int64("walks", 20_000, "pool search: walks per level before plus moves")
	fs.Parse(args)
	if *pool > 0 {
		sl, err := parseSeeds(*seeds)
		if err != nil {
			die(err)
		}
		os.MkdirAll(*outDir, 0o755)
		t0 := time.Now()
		fmt.Printf("pool search <%d,%d,%d> pool=%d walk=%d walks=%d max=%d\n", *n, *m, *p, *pool, *walkLen, *walks, *maxAbs)
		best := matmul.SearchPool(matmul.Standard(*n, *m, *p), matmul.PoolParams{
			Workers: *workers, MaxAbs: int8(*maxAbs), PoolSize: *pool, WalkLen: *walkLen,
			Walks: *walks, PlusTry: *walks, Target: *target, Seed: sl[0],
		}, func(s *matmul.Scheme, size int) {
			if !s.Correct() {
				die(fmt.Errorf("pool search produced an incorrect scheme (bug)"))
			}
			path := filepath.Join(*outDir, fmt.Sprintf("mm-%d-%d-%d-r%d-pool.txt", *n, *m, *p, len(s.Terms)))
			os.WriteFile(path, []byte(s.String()), 0o644)
			fmt.Printf("  rank %d (pool %d) (%s)\n", len(s.Terms), size, time.Since(t0).Round(time.Second))
		})
		fmt.Printf("<%d,%d,%d>: best rank %d, wall %s\n", *n, *m, *p, len(best.Terms), time.Since(t0).Round(time.Second))
		return
	}
	sl, err := parseSeeds(*seeds)
	if err != nil {
		die(err)
	}
	os.MkdirAll(*outDir, 0o755)
	os.MkdirAll(filepath.Dir(*ledgerPath), 0o755)
	l, err := run.OpenLedger(*ledgerPath)
	if err != nil {
		die(err)
	}
	defer l.Close()
	job := fmt.Sprintf("matmul <%d,%d,%d> flip max=%d steps=%d stall=%d restart=%d", *n, *m, *p, *maxAbs, *steps, *stall, *restart)
	fmt.Println("job:", job)
	t0 := time.Now()
	var mu sync.Mutex
	bestAll := *n * *m * *p
	recs := run.Seeds(l, job, sl, *workers, func(seed uint64) run.Record {
		var artifact string
		save := func(s *matmul.Scheme) {
			if !s.Correct() {
				die(fmt.Errorf("seed %d: search produced an incorrect scheme (bug)", seed))
			}
			path := filepath.Join(*outDir, fmt.Sprintf("mm-%d-%d-%d-r%d-seed%d.txt", *n, *m, *p, len(s.Terms), seed))
			os.WriteFile(path, []byte(s.String()), 0o644)
			artifact = path
			mu.Lock()
			if len(s.Terms) < bestAll {
				bestAll = len(s.Terms)
				fmt.Printf("  seed %d: rank %d (%s)\n", seed, bestAll, time.Since(t0).Round(time.Second))
			}
			mu.Unlock()
		}
		res := matmul.Search(matmul.Standard(*n, *m, *p), matmul.Params{
			Steps: *steps, MaxAbs: int8(*maxAbs), Stall: *stall, Seed: seed, Target: *target, Restart: *restart,
		}, save)
		return run.Record{Best: len(res.Best.Terms), Moves: res.Steps, Artifact: artifact}
	})
	best := bestAll
	for _, r := range recs {
		if r.Best < best {
			best = r.Best
		}
	}
	fmt.Printf("<%d,%d,%d>: best rank %d over %d seeds, wall %s\n", *n, *m, *p, best, len(recs), time.Since(t0).Round(time.Second))
}

func createOrStdout(path string) (*os.File, error) {
	if path == "-" {
		return os.Stdout, nil
	}
	return os.Create(path)
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
