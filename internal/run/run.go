// Package run spreads independent seeds across worker goroutines, logs
// every finished seed to an append-only JSONL ledger, and skips seeds the
// ledger already has, so a stopped job resumes where it left off.
package run

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// Record is one ledger line.
type Record struct {
	Job       string  `json:"job"`
	Seed      uint64  `json:"seed"`
	Solved    bool    `json:"solved"`
	Verified  bool    `json:"verified"`
	Best      int     `json:"best"`
	Moves     int64   `json:"moves"`
	WallMS    int64   `json:"wall_ms"`
	MovesPerS float64 `json:"moves_per_s"`
	Artifact  string  `json:"artifact,omitempty"`
	At        string  `json:"at"`
}

// Ledger is an append-only JSONL file.
type Ledger struct {
	mu   sync.Mutex
	f    *os.File
	done map[string]map[uint64]bool
}

// OpenLedger opens or creates path and loads the seeds it already holds.
func OpenLedger(path string) (*Ledger, error) {
	l := &Ledger{done: map[string]map[uint64]bool{}}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var r Record
			if json.Unmarshal(sc.Bytes(), &r) == nil {
				l.mark(r.Job, r.Seed)
			}
		}
		f.Close()
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	l.f = f
	return l, nil
}

func (l *Ledger) mark(job string, seed uint64) {
	if l.done[job] == nil {
		l.done[job] = map[uint64]bool{}
	}
	l.done[job][seed] = true
}

// Done reports whether job already finished seed.
func (l *Ledger) Done(job string, seed uint64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.done[job][seed]
}

// Write appends a record and flushes it to disk.
func (l *Ledger) Write(r Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	r.At = time.Now().UTC().Format(time.RFC3339)
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if _, err := l.f.Write(append(b, '\n')); err != nil {
		return err
	}
	l.mark(r.Job, r.Seed)
	return l.f.Sync()
}

// Close closes the file.
func (l *Ledger) Close() error { return l.f.Close() }

// Seeds runs fn for every seed not yet in the ledger, workers at a time.
// fn returns a filled Record; Seeds adds timing and writes it.
func Seeds(l *Ledger, job string, seeds []uint64, workers int, fn func(seed uint64) Record) []Record {
	ch := make(chan uint64)
	go func() {
		for _, s := range seeds {
			if !l.Done(job, s) {
				ch <- s
			}
		}
		close(ch)
	}()
	var mu sync.Mutex
	var out []Record
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range ch {
				t0 := time.Now()
				r := fn(s)
				r.Job, r.Seed = job, s
				r.WallMS = time.Since(t0).Milliseconds()
				if r.WallMS > 0 {
					r.MovesPerS = float64(r.Moves) / (float64(r.WallMS) / 1000)
				}
				if err := l.Write(r); err != nil {
					panic(err)
				}
				mu.Lock()
				out = append(out, r)
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return out
}
