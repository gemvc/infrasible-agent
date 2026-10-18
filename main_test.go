package main

import "testing"

func TestUsage(t *testing.T) {
	if run(nil) == nil {
		t.Fatal("expected usage error")
	}
}
