# mathbench

A small, fast Go workbench for computer search in combinatorics. It came
out of a Bullpen debate on how to do real math discovery on one M1 Max.
The rules that debate settled on are built in:

- **The LLM writes searchers. Go does the searching.** No model proposes
  answers one at a time.
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

The first target is the covering array CA(41; 3, 8, 3). See
[FINDINGS.md](FINDINGS.md).

## Commands

```bash
go build -o wb ./cmd/wb
./wb verify internal/ca/testdata/CA_42_3_8_3.txt
./wb swap -m 6 internal/ca/testdata/CA_42_3_8_3.txt
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
- `internal/engine`: incremental coverage counts (about 8 million cell
  updates per second per core) and simulated annealing driven by
  uncovered requirements.
- `internal/run`: worker pool and JSONL ledger.
- `cmd/wb`: the CLI.

## Checks

```bash
go vet ./... && go test ./...
```
