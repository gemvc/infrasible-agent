package main

import (
	"os"
	"strings"
	"testing"
)

// TestAgentVersionMatchesVERSIONMD keeps VERSION.md as the single semver source:
// scripts/ci-cd.sh writes that file; version.go must match it.
func TestAgentVersionMatchesVERSIONMD(t *testing.T) {
	raw, err := os.ReadFile("VERSION.md")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(raw))
	if want == "" {
		t.Fatal("VERSION.md is empty")
	}
	if agentVersion != want {
		t.Fatalf("agentVersion=%q does not match VERSION.md=%q", agentVersion, want)
	}
}

func TestProtocolVersion(t *testing.T) {
	if protocolMajor != 2 || protocolMinor != 0 {
		t.Fatalf("unexpected protocol version: %d.%d", protocolMajor, protocolMinor)
	}
}
