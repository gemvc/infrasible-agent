package main

import "testing"

func TestAgentVersion(t *testing.T) {
	if agentVersion != "0.3.0" {
		t.Fatalf("unexpected agent version: %s", agentVersion)
	}
}

func TestProtocolVersion(t *testing.T) {
	if protocolMajor != 2 || protocolMinor != 0 {
		t.Fatalf("unexpected protocol version: %d.%d", protocolMajor, protocolMinor)
	}
}
