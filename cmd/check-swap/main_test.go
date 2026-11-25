package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const array = "../../internal/ca/testdata/CA_42_3_8_3.txt"

func TestImportsOnlyStdlib(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		if p := strings.Trim(imp.Path.Value, `"`); strings.Contains(p, ".") {
			t.Fatalf("check-swap imports %s; it may only use the standard library", p)
		}
	}
}

// certs runs `wb swap -m m -certs` and returns the certificate bytes.
func certs(t *testing.T, m string) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "certs.bin")
	cmd := exec.Command("go", "run", "../wb", "swap", "-m", m, "-certs", path, "-out", t.TempDir(), array)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wb swap: %v\n%s", err, out)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func check(t *testing.T, b []byte) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.bin")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return run(array, path, io.Discard)
}

func TestAcceptsRealCertificates(t *testing.T) {
	for _, m := range []string{"3", "4"} {
		if err := check(t, certs(t, m)); err != nil {
			t.Fatalf("m=%s: %v", m, err)
		}
	}
}

// Damage must be caught: a changed requirement, a missing record, a
// duplicated record, and a header for a different array.
func TestRejectsDamage(t *testing.T) {
	good := certs(t, "4")
	nl := bytes.IndexByte(good, '\n') + 1
	recLen := 8 + 4*2*3
	body := good[nl:]

	bad := map[string][]byte{}
	// Flip each value byte of the first record's first requirement.
	for j := 0; j < 3; j++ {
		b := bytes.Clone(good)
		b[nl+8+3+j] = (b[nl+8+3+j] + 1) % 3
		bad["value"+string(rune('0'+j))] = b
	}
	bad["missing"] = good[:len(good)-recLen]
	bad["duplicate"] = append(append(bytes.Clone(good[:nl]), body[:recLen]...), body[:len(body)-recLen]...)
	bad["header"] = append([]byte(strings.Replace(string(good[:nl]), "sha256=d", "sha256=e", 1)), body...)
	for name, b := range bad {
		if err := check(t, b); err == nil {
			t.Errorf("%s: damaged certificates accepted", name)
		}
	}
}
