package matmul

// Params configures one flip graph run.
type Params struct {
	Steps   int64 // flip attempts
	MaxAbs  int8  // coefficient bound
	Stall   int64 // flips without progress before a plus move
	Seed    uint64
	Target  int // stop once the rank reaches this (0: never)
	Restart int64
}

// Result is the best scheme a run found.
type Result struct {
	Best  *Scheme
	Steps int64
	Plus  int
}

// Search walks the flip graph from start, keeping the shortest scheme
// seen. Plus moves are only made from the best level, and a walk that
// wanders more than one term above the best for Restart steps goes back
// to the best scheme.
func Search(start *Scheme, p Params, onBest func(*Scheme)) Result {
	w := NewWalker(start.Clone(), p.MaxAbs, p.Seed)
	best := w.S.Clone()
	var stall, above int64
	res := Result{}
	for res.Steps = 0; res.Steps < p.Steps; res.Steps++ {
		if p.Target > 0 && len(best.Terms) <= p.Target {
			break
		}
		w.Flip()
		r := w.Rank()
		switch {
		case r < len(best.Terms):
			best = w.S.Clone()
			stall, above = 0, 0
			if onBest != nil {
				onBest(best)
			}
			continue
		case r > len(best.Terms):
			above++
		default:
			above = 0
		}
		stall++
		if stall > p.Stall && r == len(best.Terms) {
			if w.Plus() {
				res.Plus++
			}
			stall = 0
		}
		if p.Restart > 0 && above > p.Restart {
			w = NewWalker(best.Clone(), p.MaxAbs, p.Seed+uint64(res.Steps))
			above, stall = 0, 0
		}
	}
	res.Best = best
	return res
}
