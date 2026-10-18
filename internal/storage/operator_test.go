package storage

import (
	"database/sql"
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

func TestOperatorInspectionRejectsExistingDatabaseWithoutIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "incomplete.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE unrelated (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenExisting(path); err == nil {
		t.Fatal("operator inspection silently registered a new identity")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err = db.QueryRow("SELECT count(*) FROM server_info").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("operator inspection bootstrapped %d identity records", count)
	}
}
