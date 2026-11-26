package matmul

import (
	"crypto/sha256"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// PoolParams configures a level-by-level pool search.
type PoolParams struct {
	Workers  int
	MaxAbs   int8
	PoolSize int   // schemes kept per level
	WalkLen  int64 // flips per walk before giving up on it
	Walks    int64 // walks per level before trying plus moves
	PlusTry  int64 // walks per level that start with a plus move
	Target   int
	Seed     uint64
}

// fingerprint identifies a scheme up to term order and signs.
func fingerprint(s *Scheme) string {
	keys := make([]string, len(s.Terms))
	for i, t := range s.Terms {
		var b strings.Builder
		for a := 0; a < 3; a++ {
			k, _ := key(t[a])
			b.WriteString(k)
			b.WriteByte('|')
		}
		keys[i] = b.String()
	}
	sort.Strings(keys)
	h := sha256.Sum256([]byte(strings.Join(keys, "/")))
	return string(h[:])
}

// SearchPool improves start one level at a time. At each level it runs
// walks from random members of the pool; any walk that finds a shorter
// scheme adds it to the next pool. If plain walks find nothing, walks
// start with a plus move instead (one term longer, more shared factors).
// onLevel is called with one scheme for each new level reached.
func SearchPool(start *Scheme, p PoolParams, onLevel func(*Scheme, int)) *Scheme {
	pool := []*Scheme{start.Clone()}
	level := len(start.Terms)
	best := start.Clone()
	var counter atomic.Uint64
	for p.Target == 0 || level > p.Target {
		next := map[string]*Scheme{}
		var mu sync.Mutex
		var walks atomic.Int64
		run := func(plus bool, limit int64) {
			var wg sync.WaitGroup
			for wk := 0; wk < p.Workers; wk++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					rng := rand.New(rand.NewPCG(p.Seed, counter.Add(1)))
					for walks.Add(1) <= limit {
						mu.Lock()
						done := len(next) >= p.PoolSize
						mu.Unlock()
						if done {
							return
						}
						src := pool[rng.IntN(len(pool))]
						w := NewWalker(src.Clone(), p.MaxAbs, rng.Uint64())
						if plus {
							for tries := 0; tries < 20 && !w.Plus(); tries++ {
							}
						}
						for f := int64(0); f < p.WalkLen; f++ {
							w.Flip()
							if w.Rank() < level {
								s := w.S.Clone()
								mu.Lock()
								next[fingerprint(s)] = s
								mu.Unlock()
								break
							}
						}
					}
				}()
			}
			wg.Wait()
		}
		run(false, p.Walks)
		if len(next) == 0 {
			walks.Store(0)
			run(true, p.PlusTry)
		}
		if len(next) == 0 {
			return best
		}
		// The next level may skip ranks (a walk can drop more than one).
		newLevel := level
		for _, s := range next {
			if len(s.Terms) < newLevel {
				newLevel = len(s.Terms)
			}
		}
		pool = pool[:0]
		for _, s := range next {
			if len(s.Terms) == newLevel {
				pool = append(pool, s)
			}
		}
		level = newLevel
		best = pool[0]
		if onLevel != nil {
			onLevel(best, len(pool))
		}
	}
	return best
}
