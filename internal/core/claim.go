// Package core holds what every domain shares: claims and how their
// status is computed. Search code, state and verifiers live in domains.
//
// A claim is one mathematical statement with the evidence for it. Its
// status is computed from that evidence, never typed in:
//
//   - "confirmed": a standalone checker (standard library only, no code
//     shared with the search) checked the certificate, or two independent
//     implementations agree; and every premise is confirmed.
//   - "checked except ...": all machine checks pass, but the result still
//     rests on the listed items: prose lemmas no machine has checked,
//     published theorems taken on trust, or an enumeration with only one
//     implementation.
//   - "claim": nothing has checked it.
//
// The program never decides that a result is new. Literature checks are
// recorded with their date and shown as data.
package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Claim is one file in claims/.
type Claim struct {
	ID        string   `json:"id"`
	Statement string   `json:"statement"`
	Kind      string   `json:"kind"` // exists | none | count | bound
	Scope     string   `json:"scope,omitempty"`
	Cert      Cert     `json:"cert"`
	Premises  []string `json:"premises,omitempty"`
	// Lemmas are proof steps argued in prose that no machine has checked.
	Lemmas []string `json:"lemmas,omitempty"`
	// Outside are published results used without re-proving them.
	Outside    []string    `json:"outside,omitempty"`
	Literature *Literature `json:"literature,omitempty"`
}

// Cert says what backs a claim and how to re-check it.
type Cert struct {
	Type string `json:"type"` // witness | enum | clique-certs | lrat | derived
	// Checker is "standalone" (a stdlib-only checker re-checked the
	// certificate), "in-repo" (repo code computed it, under tests), or
	// "none".
	Checker string `json:"checker"`
	// Independent names a second implementation or published result that
	// agrees. Empty means only one implementation exists.
	Independent string `json:"independent,omitempty"`
	// Recheck commands, run from the repo root, must all exit 0.
	Recheck []string `json:"recheck,omitempty"`
	// SlowRecheck commands take minutes; they run only when asked.
	SlowRecheck []string `json:"slow_recheck,omitempty"`
	Artifact    string   `json:"artifact,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
}

// Literature records the last check for earlier results.
type Literature struct {
	Checked string `json:"checked"` // date
	Best    string `json:"best_published"`
	Notes   string `json:"notes,omitempty"`
}

// Load reads every claims/*.json file.
func Load(dir string) (map[string]*Claim, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := map[string]*Claim{}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var c Claim
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return nil, fmt.Errorf("%s: %v", f, err)
		}
		if c.ID+".json" != filepath.Base(f) {
			return nil, fmt.Errorf("%s: id %q does not match the file name", f, c.ID)
		}
		out[c.ID] = &c
	}
	return out, nil
}

// Status is a computed verdict.
type Status struct {
	Level   string   // confirmed | checked-except | claim
	Reasons []string // what keeps it from "confirmed"
}

func (s Status) String() string {
	switch s.Level {
	case "confirmed":
		return "confirmed"
	case "claim":
		return "claim (not checked)"
	}
	return "checked except: " + strings.Join(s.Reasons, "; ")
}

// Compute works out the status of claim id from its evidence and the
// status of its premises.
func Compute(all map[string]*Claim, id string) (Status, error) {
	return compute(all, id, map[string]bool{})
}

func compute(all map[string]*Claim, id string, visiting map[string]bool) (Status, error) {
	c, ok := all[id]
	if !ok {
		return Status{}, fmt.Errorf("unknown claim %q", id)
	}
	if visiting[id] {
		return Status{}, fmt.Errorf("claim %q depends on itself", id)
	}
	visiting[id] = true
	defer delete(visiting, id)

	reasons := map[string]bool{}
	unchecked := false
	switch c.Cert.Checker {
	case "standalone":
	case "in-repo":
		if c.Cert.Independent == "" && c.Cert.Type != "derived" {
			reasons["single implementation: "+id] = true
		}
	case "none", "":
		unchecked = true
	default:
		return Status{}, fmt.Errorf("claim %q: unknown checker %q", id, c.Cert.Checker)
	}
	for _, l := range c.Lemmas {
		reasons["unchecked lemma: "+l] = true
	}
	for _, o := range c.Outside {
		reasons["published result taken on trust: "+o] = true
	}
	for _, p := range c.Premises {
		ps, err := compute(all, p, visiting)
		if err != nil {
			return Status{}, err
		}
		if ps.Level == "claim" {
			unchecked = true
		}
		for _, r := range ps.Reasons {
			reasons[r] = true
		}
	}
	if unchecked {
		return Status{Level: "claim"}, nil
	}
	if len(reasons) == 0 {
		return Status{Level: "confirmed"}, nil
	}
	var rs []string
	for r := range reasons {
		rs = append(rs, r)
	}
	sort.Strings(rs)
	return Status{Level: "checked-except", Reasons: rs}, nil
}
