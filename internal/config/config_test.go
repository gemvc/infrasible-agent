package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRejectUntrusted(t *testing.T) {
	cases := []Config{
		{ConfigVersion: 2},
		{ConfigVersion: 1, MonitoringServers: []Server{{ID: "a", LiveURL: "ws://evil", ServerName: "host", CAFile: "ca", ClientCertFile: "cert", ClientKeyFile: "key"}}},
		{ConfigVersion: 1, MonitoringServers: []Server{{ID: "a", LiveURL: "wss://host", ServerName: "host", CAFile: "ca", ClientCertFile: "cert", ClientKeyFile: "key"}, {ID: "a", LiveURL: "wss://host", ServerName: "host", CAFile: "ca", ClientCertFile: "cert", ClientKeyFile: "key"}}},
	}
	for _, c := range cases {
		if e := c.Validate(); e == nil {
			t.Fatalf("accepted %+v", c)
		}
	}
}

func TestLoadHappyPathAndDefaultDataPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"config_version":1,"monitoring_servers":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.DataPath != "/var/lib/infrasible-agent/agent.db" {
		t.Fatalf("default data_path: %q", c.DataPath)
	}
	if c.ConfigVersion != 1 || len(c.MonitoringServers) != 0 {
		t.Fatalf("unexpected config: %+v", c)
	}
	if len(c.EnabledFamilies) != 6 {
		t.Fatalf("default enabled_families: %v", c.EnabledFamilies)
	}
}

func TestLoadEnabledFamiliesSubset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"config_version":1,"enabled_families":["cpu","load"],"monitoring_servers":[]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.EnabledFamilies) != 2 || c.EnabledFamilies[0] != "cpu" || c.EnabledFamilies[1] != "load" {
		t.Fatalf("enabled_families: %v", c.EnabledFamilies)
	}
}

func TestLoadRejectsEmptyOrUnknownOrDuplicateFamilies(t *testing.T) {
	dir := t.TempDir()
	cases := []string{
		`{"config_version":1,"enabled_families":[],"monitoring_servers":[]}`,
		`{"config_version":1,"enabled_families":["cpu","gpu"],"monitoring_servers":[]}`,
		`{"config_version":1,"enabled_families":["cpu","cpu"],"monitoring_servers":[]}`,
	}
	for i, body := range cases {
		path := filepath.Join(dir, fmt.Sprintf("bad-%d.json", i))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted invalid enabled_families: %s", body)
		}
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"config_version":1,"monitoring_servers":[]}{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("expected trailing JSON rejection")
	}
}

func TestValidateAcceptsSecuredServer(t *testing.T) {
	c := Config{
		ConfigVersion: 1,
		DataPath:      "/tmp/agent.db",
		MonitoringServers: []Server{{
			ID: "primary", LiveURL: "wss://bridge.example/live", PersistenceURL: "https://api.example/persist",
			PersistenceDomain: "domain-a", ServerName: "bridge.example",
			CAFile: "/etc/ca.pem", ClientCertFile: "/etc/cert.pem", ClientKeyFile: "/etc/key.pem",
		}},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsMissingTLSAndDomain(t *testing.T) {
	missingTLS := Config{
		ConfigVersion: 1, DataPath: "/tmp/agent.db",
		MonitoringServers: []Server{{ID: "a", LiveURL: "wss://host.example", ServerName: "host.example"}},
	}
	if err := missingTLS.Validate(); err == nil {
		t.Fatal("expected missing TLS identity")
	}
	missingDomain := Config{
		ConfigVersion: 1, DataPath: "/tmp/agent.db",
		MonitoringServers: []Server{{
			ID: "a", PersistenceURL: "https://api.example", ServerName: "api.example",
			CAFile: "ca", ClientCertFile: "cert", ClientKeyFile: "key",
		}},
	}
	if err := missingDomain.Validate(); err == nil {
		t.Fatal("expected missing persistence domain")
	}
}

func TestIntervalIsOneSecond(t *testing.T) {
	if Interval() != time.Second {
		t.Fatalf("Interval=%v", Interval())
	}
}
