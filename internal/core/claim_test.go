package core

import (
	"strings"
	"testing"
)

func TestCompute(t *testing.T) {
	all := map[string]*Claim{
		"base":    {ID: "base", Cert: Cert{Checker: "standalone"}},
		"enum":    {ID: "enum", Cert: Cert{Checker: "in-repo"}},
		"paired":  {ID: "paired", Cert: Cert{Checker: "in-repo", Independent: "paper table"}},
		"lemma":   {ID: "lemma", Cert: Cert{Checker: "standalone"}, Lemmas: []string{"pigeonhole"}},
		"derived": {ID: "derived", Cert: Cert{Type: "derived", Checker: "in-repo"}, Premises: []string{"enum", "lemma"}},
		"open":    {ID: "open", Cert: Cert{Checker: "none"}},
		"onopen":  {ID: "onopen", Cert: Cert{Checker: "standalone"}, Premises: []string{"open"}},
		"loop":    {ID: "loop", Cert: Cert{Checker: "standalone"}, Premises: []string{"loop"}},
	}
	want := map[string]string{
		"base":    "confirmed",
		"paired":  "confirmed",
		"enum":    "checked except: single implementation: enum",
		"lemma":   "checked except: unchecked lemma: pigeonhole",
		"derived": "checked except: single implementation: enum; unchecked lemma: pigeonhole",
		"open":    "claim (not checked)",
		"onopen":  "claim (not checked)",
	}
	for id, w := range want {
		s, err := Compute(all, id)
		if err != nil {
			t.Fatal(err)
		}
		if s.String() != w {
			t.Errorf("%s: %q, want %q", id, s, w)
		}
	}
	if _, err := Compute(all, "loop"); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Errorf("self-dependency not caught: %v", err)
	}
}

// The real claims folder must load, and every claim must get a status.
func TestRealClaims(t *testing.T) {
	all, err := Load("../../claims")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) == 0 {
		t.Fatal("no claims")
	}
	for id := range all {
		if _, err := Compute(all, id); err != nil {
			t.Error(err)
		}
	}
}
