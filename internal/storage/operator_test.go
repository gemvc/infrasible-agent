package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorInventoryDoesNotBootstrapIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.db")
	if _, e := OpenExisting(path); e == nil {
		t.Fatal("operator status created a missing database")
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatalf("storage unexpectedly created: %v", e)
	}
	real := filepath.Join(dir, "real.db")
	s, e := Open(real)
	if e != nil {
		t.Fatal(e)
	}
	s.Close()
	link := filepath.Join(dir, "link.db")
	if e = os.Symlink(real, link); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenExisting(link); e == nil {
		t.Fatal("symlink database accepted")
	}
	reopened, e := OpenExisting(real)
	if e != nil {
		t.Fatal(e)
	}
	reopened.Close()
}
