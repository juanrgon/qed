# Findings

Date: 2026-11-25. Target: CA(N; 3, 8, 3), a strength-3 covering array on
8 columns and 3 symbols. The live table at coveringarraytools.org (build
dated 2026-11-24) gives 37 <= CAN(3, 8, 3) <= 42.

Status labels: **confirmed** means rerunnable code in this repo proves it.
**Reproduced** means this repo independently matches someone else's
numbers. **Claim** means not yet checked.

## 1. Likely new: CAN(3, 8, 3) >= 39 (no 37- or 38-row array exists)

Reproduced, plus one outside fact. Not peer reviewed. Not posted anywhere.

**Argument.** In a CA(N; 3, 8, 3), fix a column j and symbol s. The rows
with column j equal to s must cover every pair on the other 7 columns, so
they form a CA(n; 2, 7, 3). Since CAN(2, 7, 3) = 12, each such "slice"
has at least 12 rows. If N <= 38, the three symbols of any column cannot
all have 13 or more rows, so every column has a "light" symbol with
exactly 12 rows. A light slice is then a CA(12; 2, 7, 3).

CA(12; 2, 7, 3) is unique up to isomorphism (outside fact, see below).
Relabel so column 0's light slice A is a fixed copy R of it. For column Y
with light symbol v, the slice B = {rows with Y = v} has 12 rows, holds
exactly A's rows with Y = v (every row with column 0 = 0 is in A), and
covers all pairs on the other 7 columns. For a second column Z with light
symbol w, the slice C must satisfy {b in B : b[Z] = w} = {c in C : c[Y] =
v} as multisets, because both are the array's rows with Y = v and Z = w.

`wb lightslice` lists every possible B for each (Y, v) by direct search and
counts compatible (B, C) pairs. With R = the table's representative, 6 of
the 21 column pairs have zero compatible pairs for all 9 (v, w), for
example columns (1, 4). Since columns 1 and 4 must both have light
symbols, no CA(38; 3, 8, 3) exists. Runtime: about 4 seconds.

**Independence.** The argument came from the Bullpen debate (Significance
Judge, then Feasibility Engineer). This repo's check uses a different
method from the panelists': it builds slices column by column from the
covering rule alone, with no isomorphism code and no balance assumption.
It matches their numbers exactly:

- Slice counts 4 / 96 / 96 and 8 / 8 / 8 per column (panel: 96, 4, 8).
- Table copy: obstructions (1,4) (1,5) (3,6) (3,7) (4,5) (6,7). This
  matches the Feasibility Engineer's list.
- Kokkala copy: obstructions (1,5) (1,6) (2,4) (2,7) (4,7) (5,6), and the
  control pair (1,2) gives the diagonal (4, 2, 4). This matches the
  Problem Scout's list and the Judge's matrix.

Tests: every slice found passes the plain verifier, and on 256 hand-built
slices per (Y, v) the search agrees with the verifier both ways (42 valid
ones, all found).

**Outside fact this depends on.** CA(12; 2, 7, 3) is unique, and
CAN(2, 7, 3) = 12. Sources: Colbourn, Kéri, Rivas Soriano, Schlage-Puchta
(2010) and Kokkala et al., [arXiv 1901.03594](https://arxiv.org/abs/1901.03594).
Not re-proved here.

**Novelty check.**
- The live table (2026-11-24) still says 37.
- [Izquierdo-Marquez and Torres-Jimenez 2019](https://arxiv.org/abs/1711.10040)
  handles 7 and 9 columns and skips 8.
- Two web searches found no bound of 38 or 39.
- Not yet checked: their 2018 orderly-algorithm paper
  (doi:10.1142/S1793830918500118), and Colbourn's own tables (login wall).

**Corollaries.** CAN(t, k, v) >= v * CAN(t-1, k-1, v) gives
CAN(4, 9, 3) >= 117 and CAN(5, 10, 3) >= 351 (the panel says the tables
show 111 and 333; not checked here).

**Before calling it settled:**
1. Re-prove uniqueness of CA(12; 2, 7, 3), for example with a SAT solver.
2. Check the light-slice step with a third, direct SAT encoding.
3. Read the 2018 paper and Colbourn's tables.
4. Have a human combinatorialist read the argument.

## 2. Row swaps from the published 42-row array are dead

Confirmed. `wb swap -m M` checks every M-subset of the published
CA(42; 3, 8, 3) (sha256 d92e84fc...) and proves that M rows cannot be
replaced by M - 1. Every case was certified by a clique of M pairwise
conflicting requirements. There were no undetermined cases.

| M -> M-1 | subsets | time (8 cores) | result |
|---|---|---|---|
| 3 -> 2 | 11,480 | <1 s | impossible (reproduces panel) |
| 4 -> 3 | 111,930 | <1 s | impossible (reproduces panel) |
| 5 -> 4 | 850,668 | ~3 s | impossible (reproduces panel) |
| 6 -> 5 | 5,245,786 | 29 s | impossible (new) |
| 7 -> 6 | 26,978,328 | 3 m 35 s | impossible (new) |

So any 41-row array keeps at most 34 of the published 42 rows.

## 3. Structure beats tuning (search results)

Confirmed on this machine, with 8 seeds per configuration.

- The published 42-row array is invariant under the symbol shift
  x -> x + 1 mod 3 (test in `internal/engine`).
- **Plain annealing never reaches 42 rows.** It gets best 15 to 18
  missing, at several temperatures, with about 30M moves per seed.
- **Shift-symmetric annealing rebuilds a 42-row array in 8 of 8 seeds in
  about 9 seconds** (`-model shift -reps 14 -t0 3 -t1 0.3`). That gives
  verified arrays in `runs/solutions/`. Caveat: the symmetry was read off
  the published array, so this is replication, not invention.
- Temperature must scale with how many cells a move touches. The same
  shift model at T = 1..0.05 never solves.
- Column-cycle groups (c7 with 6 orbits, ck with 5 orbits + 2 free) did
  not rebuild 42 on a short budget.
- At 45 rows the plain engine fails like the panel's naive annealer did
  (0 of 8 solved, best 1 to 2 missing).

## 4. Dead ends for 41 rows, with reasons

- **Shift orbits + 2 free rows (13 + 2): impossible by design.** The
  shift sends each requirement to two others on the same columns, and one
  row covers at most one of the three. So any gap left by the orbits needs
  3 free rows. All 8 seeds stalled at exactly 18 missing. Note that the
  debate's Feasibility Engineer still suggests this run. It is a waste.
- **Shift orbits + 5 free rows (12 + 5):** all 40 seeds stalled at
  exactly 20 missing, which points to another structural floor. Not yet
  explained.
- **Fixing column 0's symbol split** (the first debate's "five splits")
  loses nothing, but it also gains almost nothing over plain search. It
  is not the structural lever it sounded like.

## Open next steps

1. Firm up result 1 (see the list above), then write a short note.
   Posting it or contacting the table maintainer is a public step that
   needs Juan's OK.
2. CA(39; 3, 8, 3): with every column 13/13/13 now forced, each slice is
   a CA(13; 2, 7, 3), and there are 24,934 classes of those. The same
   slice method could settle 39, which would make 40 the bound.
3. For 41 rows: try groups that do not trap free rows, and enumerate tiny
   models exactly (c7z3 at 42 has only 16 variables).
