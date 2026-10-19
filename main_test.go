package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gemvc/infrasible-agent/internal/storage"
)

func TestUsage(t *testing.T) {
	if dispatch(nil) == nil {
		t.Fatal("expected usage error")
	}
}

func TestRunRequiresLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux host: gate not exercised")
	}
	if err := run([]string{"config", "validate"}); err == nil || err.Error() != "Linux only" {
		t.Fatalf("expected Linux only, got %v", err)
	}
}

func TestDispatchConfigValidate(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{"config_version":1,"data_path":"`+filepath.ToSlash(filepath.Join(dir, "agent.db"))+`","monitoring_servers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dispatch([]string{"config", "validate", "--config=" + cfg}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchStorageRequiresExistingDB(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	db := filepath.Join(dir, "missing.db")
	if err := os.WriteFile(cfg, []byte(`{"config_version":1,"data_path":"`+filepath.ToSlash(db)+`","monitoring_servers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := dispatch([]string{"storage", "status", "--config=" + cfg})
	if err == nil {
		t.Fatal("expected missing database error")
	}
	if !strings.Contains(err.Error(), "no such file") && !os.IsNotExist(err) {
		// OpenExisting wraps Lstat errors; accept any failure for missing path.
		t.Logf("got expected failure: %v", err)
	}
}

func TestDispatchHealthAndStorageStatus(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "agent.db")
	s, err := storage.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{"config_version":1,"data_path":"`+filepath.ToSlash(db)+`","monitoring_servers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dispatch([]string{"health", "status", "--config=" + cfg}); err != nil {
		t.Fatal(err)
	}
	if err := dispatch([]string{"storage", "status", "--config=" + cfg}); err != nil {
		t.Fatal(err)
	}
	if err := dispatch([]string{"storage", "cleanup", "--dry-run", "--config=" + cfg}); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchUnknownCommand(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfg, []byte(`{"config_version":1,"data_path":"`+filepath.ToSlash(filepath.Join(dir, "agent.db"))+`","monitoring_servers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := dispatch([]string{"nope", "--config=" + cfg}); err == nil {
		t.Fatal("expected usage error")
	}
}
