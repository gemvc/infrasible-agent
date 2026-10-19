package collector

import (
	"context"
	"testing"
	"time"
)

func TestNewFromNamesFiltersFamilies(t *testing.T) {
	r, err := NewFromNames([]string{"cpu", "load"})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Names(); len(got) != 2 || got[0] != "cpu" || got[1] != "load" {
		t.Fatalf("names: %v", got)
	}
	profile := r.CadenceProfile()
	if profile["cpu"] != 1 || profile["load"] != 5 || len(profile) != 2 {
		t.Fatalf("cadence: %#v", profile)
	}
	f := r.Read(context.Background(), time.Unix(1, 0).UTC())
	if _, ok := f.Metrics["cpu"]; !ok {
		t.Fatal("cpu missing when enabled")
	}
	if _, ok := f.Metrics["load"]; ok {
		t.Fatal("load present when not due")
	}
	if _, ok := f.Metrics["memory"]; ok {
		t.Fatal("disabled memory appeared in frame")
	}
}

func TestNewFromNamesRejectsUnknown(t *testing.T) {
	if _, err := NewFromNames([]string{"cpu", "gpu"}); err == nil {
		t.Fatal("expected unknown family error")
	}
}

func TestDefaultCadenceProfileMatchesSixFamilies(t *testing.T) {
	p := DefaultCadenceProfile()
	want := map[string]int64{"cpu": 1, "memory": 1, "network": 1, "disk_io": 1, "load": 5, "filesystem": 30}
	if len(p) != len(want) {
		t.Fatalf("got %#v", p)
	}
	for k, v := range want {
		if p[k] != v {
			t.Fatalf("%s: got %d want %d", k, p[k], v)
		}
	}
}
