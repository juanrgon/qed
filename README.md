# mathbench

A small, fast Go workbench for computer search in combinatorics, built to
run on one laptop (an M1 Max). Its first target is covering arrays.

A covering array CA(N; t, k, v) is an N by k table over v symbols where
every choice of t columns shows every possible t-tuple of symbols in some
row. CAN(t, k, v) is the smallest N that works.

## Results

Status labels: **confirmed** means code in this repo proves it and you
can rerun it. **New** means confirmed here, better than any published
bound we could find, and not yet peer reviewed. Details, test controls
and the literature check are in [FINDINGS.md](FINDINGS.md).

| Result | Status | Rerun |
|---|---|---|
| CAN(3, 8, 3) >= 39. No covering array with 8 columns, 3 symbols and strength 3 has 38 or fewer rows. The best published lower bound is 37. | New (computer proof) | `./wb lightslice` (about 5 s) |
| CAN(4, 9, 3) >= 117 and CAN(5, 10, 3) >= 351, which follow from the row above. Published lower bounds are 111 and 333. | New | follows from CAN(t, k, v) >= v CAN(t-1, k-1, v) |
| In the best known CA(42; 3, 8, 3), no 6 rows can be replaced by 5 and no 7 rows by 6. So a 41-row array would keep at most 34 of its rows. | Confirmed | `./wb swap -m 7 internal/ca/testdata/CA_42_3_8_3.txt` (about 4 min on 8 cores) |
| A symmetry-aware search rebuilds a 42-row array in 8 of 8 seeds in about 9 s. Plain search did not get there in any run. | Confirmed (replication: the symmetry comes from the known array) | `./wb search -model shift -reps 14 -seeds 0:8 -moves 15000000 -t0 3 -t1 0.3` |

**What the 39 bound rests on.** If an array had 38 rows, each column
would have a symbol that appears in exactly 12 rows. Those 12 rows must
form a CA(12; 2, 7, 3) on the other columns. `wb lightslice` does it in
two steps:

1. It classifies small strength-2 arrays from scratch: no
   CA(11; 2, 7, 3) exists, and CA(12; 2, 7, 3) is unique up to relabeling.
   Both facts were published before (Colbourn, Kéri, Rivas Soriano and
   Schlage-Puchta 2010; Kokkala et al.,
   [arXiv 1901.03594](https://arxiv.org/abs/1901.03594)); this repo
   re-proves them rather than trusting them.
2. It pins column 0's 12 rows to that unique array, lists every way the
   other columns' 12-row slices can look, and finds column pairs that can
   never agree.

**Checked so far.** The classifier reproduces a published table of class counts (for
example, exactly 13 classes of CA(12; 2, 6, 3)). The slice step finds no
false contradictions on the 6-column case, where a real array exists.
Both published copies of CA(12; 2, 7, 3) are confirmed isomorphic to the
class it finds. A search of the literature through November 2026 found
no earlier proof of 38 or 39. Still open: a second, independent check
with a SAT solver, and review by a person. See [FINDINGS.md](FINDINGS.md).

## Design

- **Search code is cheap to write; Go does the searching.** Strategies are
  small Go programs, not answers proposed one at a time.
- **One plain verifier is the judge.** `internal/ca/verify.go` is naive
  and shares no code with the fast engine. A solve only counts once it
  passes there.
- **Budgets are move counts, not clocks.** The same seed always does the
  same work, whatever the laptop's temperature.
- **Structure is pluggable.** A `Model` maps search variables to array
  cells, so a strategy can freeze cells or tie cells together by symmetry
  without touching the engine.
- **Every seed is logged.** `runs/ledger.jsonl` is append-only, and a
  stopped job resumes by skipping seeds already in it.

## Commands

```bash
go build -o wb ./cmd/wb
go run ./cmd/verify-ca internal/ca/testdata/CA_42_3_8_3.txt
./wb verify internal/ca/testdata/CA_42_3_8_3.txt
./wb swap -m 6 internal/ca/testdata/CA_42_3_8_3.txt
./wb lightslice
./wb search -model shift -reps 14 -seeds 0:8 -moves 15000000 -t0 3 -t1 0.3
./wb search -model split -split 13,14,14 -seeds 0:8
./wb search -n 45 -seeds 0:8
```

Flags go before the file argument.

## Layout

- `internal/ca`: array type, strict parser, the plain verifier, and the
  requirement index.
- `internal/swap`: proves whether m rows can be replaced by m - 1. Every
  case ends as proven impossible (clique or exhaustive coloring), a
  verified witness, or undetermined.
- `internal/classify`: classifies small strength-2 arrays up to
  isomorphism, with every merge backed by a checked relabeling.
- `internal/lightslice`: the 12-row slice check behind the 39 bound.
- `internal/engine`: incremental coverage counts (about 8 million cell
  updates per second per core) and simulated annealing driven by
  uncovered requirements.
- `internal/run`: worker pool and JSONL ledger.
- `cmd/wb`: the CLI.
- `cmd/verify-ca`: the standalone judge. It uses only the Go standard
  library, so it shares no code with the search. A test enforces that.

Test arrays come from published sources; see
[internal/ca/testdata/SOURCES.md](internal/ca/testdata/SOURCES.md).

## Checks

```bash
go vet ./... && go test ./...
```

## License

MIT. See [LICENSE](LICENSE).
