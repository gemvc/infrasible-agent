package transport

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gemvc/infrasible-agent/internal/config"
)

func TestNoPlaintextEndpoint(t *testing.T) {
	c := config.Config{ConfigVersion: 1, MonitoringServers: []config.Server{{ID: "a", LiveURL: "ws://example.test", ServerName: "example.test", CAFile: "ca", ClientKeyFile: "key", ClientCertFile: "cert"}}}
	if c.Validate() == nil {
		t.Fatal("insecure WS accepted")
	}
}

func TestRejectsUnsafeMTLSPrivateKeyPermissions(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "client.key")
	if err := os.WriteFile(keyPath, []byte("invalid key"), 0644); err != nil {
		t.Fatal(err)
	}
	server := config.Server{ClientKeyFile: keyPath}
	if _, err := tlsFor(server); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("world-readable key accepted: %v", err)
	}
	if err := os.Chmod(keyPath, 0600); err != nil {
		t.Fatal(err)
	}
	// With safe permissions the key guard passes; the next error is the
	// intentionally absent CA, not a private-key permission rejection.
	if _, err := tlsFor(server); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner-only key unexpectedly rejected: %v", err)
	}
	link := filepath.Join(dir, "client-link.key")
	if err := os.Symlink(keyPath, link); err != nil {
		t.Fatal(err)
	}
	server.ClientKeyFile = link
	if _, err := tlsFor(server); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("symlink private key accepted: %v", err)
	}
}
