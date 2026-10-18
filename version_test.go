package main

import "testing"

func TestProtocolVersion(t *testing.T) {
	if agentVersion != "0.2.0" || protocolMajor != 2 || protocolMinor != 0 {
		t.Fatal("invalid v2 version")
	}
}
