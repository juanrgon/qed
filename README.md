# mathbench

A small, fast Go framework for finding and checking new results in
combinatorics on one laptop (an M1 Max). AI coding agents write the
search code; the framework decides what counts.

Every result is a **claim** with a **certificate**, and its status is
computed by the program from the evidence, not typed in by a person or an
agent. Constructions are checked by a standalone verifier. Impossibility
results need a checkable certificate: clique certificates, an exhaustive
enumeration that matches published counts, or a SAT proof (LRAT) checked
by a separate checker.

The first domain is covering arrays. Next up: graphs with no triangles or
4-cycles, and constant-weight codes (see [Roadmap](#roadmap)).

## Results

From `./wb claims` (run `./wb claims -run` to re-check them all). The
program never decides whether a result is new; the literature column is a
dated record of what we checked.

A covering array CA(N; t, k, v) is an N by k table over v symbols where
every choice of t columns shows every possible t-tuple of symbols in some
row. CAN(t, k, v) is the smallest N that works.

| Result | Computed status | Literature (checked 2026-11-25) |
|---|---|---|
| **CAN(3, 8, 3) >= 39.** No covering array with 8 columns, 3 symbols and strength 3 has 38 or fewer rows. | Checked except: one prose lemma (the light-slice reduction) and one in-repo implementation of the slice enumeration | Best published lower bound is 37. No earlier proof of 38 or 39 found; two sources still unread. |
| **CAN(4, 9, 3) >= 117 and CAN(5, 10, 3) >= 351**, from the row above. | As above, plus a standard published inequality | Published: 111 and 333. |
| **CA(12; 2, 7, 3) is unique and CAN(2, 7, 3) = 12** (premises of the 39 bound). | Confirmed (matches a published table cell by cell) | Known since 2010; re-proved here. |
| **No 7 rows of the best known CA(42; 3, 8, 3) can be replaced by 6** (nor 6 by 5, 5 by 4), so a 41-row array keeps at most 34 of its rows. | Confirmed (27 million clique certificates, checked by a standalone checker) | |
| **Symmetry-aware search rebuilds a 42-row array** in 8 of 8 seeds, about 9 s. Plain search did not in any run. | Confirmed (replication: the symmetry comes from the known array) | Record is 42. |

**What the 39 bound rests on.** If an array had 38 rows, each column
would have a symbol that appears in exactly 12 rows, and those rows form a
CA(12; 2, 7, 3) on the other columns. `./wb lightslice` (about 5 s)
classifies the small arrays from scratch, pins column 0's 12 rows to the
unique CA(12; 2, 7, 3), lists every way the other columns' slices can
look, and finds column pairs that can never agree. The prose step (why
those conditions must hold) is written out in [FINDINGS.md](FINDINGS.md);
no machine has checked it yet, which is why the status says so. The slice
step finds no false contradictions on the 6-column case, where a real
array exists.

## How it works

- **Claims** live in [`claims/`](claims/), one JSON file each: the
  statement, the certificate, premises, unchecked lemmas, published
  results taken on trust, and the literature check. `internal/core`
  computes the status: *confirmed*, *checked except ...*, or *claim*.
- **Standalone checkers** use only the Go standard library, so they share
  no code with the search. Tests enforce that.
  - `cmd/verify-ca`: is this file a covering array?
  - `cmd/check-swap`: are these row-swap certificates valid?
  - `cmd/check-lrat`: is this LRAT proof of unsatisfiability valid?
  - `cmd/verify-girth5`: does this graph have no triangle or 4-cycle?
- **No human review; the ratchet is the gate.** On every push and pull
  request, CI runs [`scripts/ratchet.sh`](scripts/ratchet.sh): the
  previous commit's checker tests (good certificates accepted, damaged
  ones rejected) must pass against the new checkers, and every claim is
  re-graded by the previous commit's checkers. Checkers can get stricter,
  never weaker, and a change cannot weaken the judge and grade its own
  claims in the same step. (Tested: weakening `verify-ca` and switching
  off its own test is caught.)
- **SAT** (`internal/sat`): CNF building and CaDiCaL with LRAT proofs.
  Every proof is checked by `cmd/check-lrat` and, when installed, also by
  [cake_lpr](https://github.com/tanyongkiam/cake_lpr), a formally
  verified checker; the two must agree. Smoke test: on 10 vertices, 15
  edges with no triangle or 4-cycle is found (the Petersen graph) and 16
  is proven impossible.
- **Search**: deterministic seeds with move budgets, an append-only
  ledger, and symmetry models that tie cells together into orbits. A
  failed search is logged as "budget exhausted", never as impossibility.

## Roadmap

From a design debate on 2026-11-25:

1. Done: SAT and LRAT pipeline, standalone checkers, swap certificates,
   claims with computed status, CI.
2. Move covering arrays into a `domains/ca` package behind a tiny core
   (claims, ledger, a `Searcher` interface), with no speed loss.
3. New domain: graphs with no triangles or 4-cycles. Reproduce the
   285-edge record on 74 vertices, then search for 286.
4. New domain: constant-weight codes. Reproduce 2,992 words for
   A(23, 6, 10), then search for 2,993.
5. Check the light-slice step with a direct SAT encoding, so the 39 bound
   no longer rests on a prose lemma.

## Commands

```bash
go build -o wb ./cmd/wb
./wb claims -run
./wb lightslice
./wb swap -m 6 internal/ca/testdata/CA_42_3_8_3.txt
./wb swap -m 5 -certs - internal/ca/testdata/CA_42_3_8_3.txt | go run ./cmd/check-swap internal/ca/testdata/CA_42_3_8_3.txt /dev/stdin
./wb search -model shift -reps 14 -seeds 0:8 -moves 15000000 -t0 3 -t1 0.3
go run ./cmd/verify-ca internal/ca/testdata/CA_42_3_8_3.txt
```

Flags go before the file argument. SAT features need `brew install
cadical`. For the second proof checker, build
[cake_lpr](https://github.com/tanyongkiam/cake_lpr) with `make` and put
it on your PATH (or set `MATHBENCH_CAKE_LPR`).

## Layout

- `claims/`: one file per result, plus small artifacts.
- `cmd/wb`: the CLI. `cmd/verify-ca`, `cmd/check-swap`, `cmd/check-lrat`:
  standalone checkers.
- `internal/core`: claims and status computation.
- `internal/sat`: CNF and CaDiCaL.
- `internal/ca`: array type, parser, plain verifier, requirement index.
- `internal/swap`: can m rows be replaced by m - 1? Clique and coloring
  proofs, with certificates.
- `internal/classify`: classifies small strength-2 arrays up to
  isomorphism, with every merge backed by a checked relabeling.
- `internal/lightslice`: the 12-row slice check behind the 39 bound.
- `internal/engine`: incremental coverage counts (about 8 million cell
  updates per second per core) and simulated annealing.
- `internal/run`: worker pool and JSONL ledger.

Test arrays come from published sources; see
[internal/ca/testdata/SOURCES.md](internal/ca/testdata/SOURCES.md).

## Checks

```bash
go vet ./... && go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
