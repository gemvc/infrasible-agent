// Package config loads and validates the single versioned Agent JSON configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
)

// Server describes one ordered Monitoring endpoint entry.
type Server struct {
	ID                string `json:"id"`
	LiveURL           string `json:"live_url"`
	PersistenceURL    string `json:"persistence_url"`
	PersistenceDomain string `json:"persistence_domain"`
	ServerName        string `json:"server_name"`
	CAFile            string `json:"ca_file"`
	ClientCertFile    string `json:"client_cert_file"`
	ClientKeyFile     string `json:"client_key_file"`
}

// Config is the sole on-disk Agent configuration snapshot.
// enabled_families is resolved at process start; changes require a daemon restart.
type Config struct {
	ConfigVersion     int      `json:"config_version"`
	DataPath          string   `json:"data_path"`
	EnabledFamilies   []string `json:"enabled_families"`
	MonitoringServers []Server `json:"monitoring_servers"`
}

// Load reads path, rejects unknown fields and trailing JSON, applies the
// default data_path and enabled_families when omitted, then Validate.
func Load(path string) (Config, error) {
	f, e := os.Open(path)
	if e != nil {
		return Config{}, e
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	var c Config
	if e = d.Decode(&c); e != nil {
		return Config{}, fmt.Errorf("decode config: %w", e)
	}
	var trailing any
	if e = d.Decode(&trailing); e == nil {
		return Config{}, errors.New("trailing JSON content")
	} else if !errors.Is(e, io.EOF) {
		return Config{}, fmt.Errorf("trailing JSON: %w", e)
	}
	if c.DataPath == "" {
		c.DataPath = "/var/lib/infrasible-agent/agent.db"
	}
	if c.EnabledFamilies == nil {
		c.EnabledFamilies = append([]string(nil), collector.DefaultFamilyNames()...)
	}
	return c, c.Validate()
}

// Validate enforces config_version, data_path, enabled_families, unique server IDs,
// TLS material paths, and secured wss/https URL schemes.
// An empty monitoring_servers list is valid.
func (c Config) Validate() error {
	if c.ConfigVersion != 1 {
		return errors.New("unsupported config_version")
	}
	if strings.TrimSpace(c.DataPath) == "" {
		return errors.New("empty data_path")
	}
	families := c.EnabledFamilies
	if families == nil {
		families = collector.DefaultFamilyNames()
	}
	if err := validateEnabledFamilies(families); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, s := range c.MonitoringServers {
		if s.ID == "" || seen[s.ID] {
			return fmt.Errorf("empty or duplicate monitoring server id: %q", s.ID)
		}
		seen[s.ID] = true
		if s.LiveURL == "" && s.PersistenceURL == "" {
			return fmt.Errorf("%s has no channel", s.ID)
		}
		if s.ServerName == "" || s.CAFile == "" || s.ClientCertFile == "" || s.ClientKeyFile == "" {
			return fmt.Errorf("%s missing TLS identity", s.ID)
		}
		for _, entry := range []struct{ raw, scheme string }{{s.LiveURL, "wss"}, {s.PersistenceURL, "https"}} {
			if entry.raw == "" {
				continue
			}
			u, e := url.Parse(entry.raw)
			if e != nil || u.Scheme != entry.scheme || u.Host == "" || u.User != nil || u.Fragment != "" {
				return fmt.Errorf("invalid secured endpoint for %s", s.ID)
			}
		}
		if s.PersistenceURL != "" && s.PersistenceDomain == "" {
			return fmt.Errorf("%s missing persistence domain", s.ID)
		}
	}
	return nil
}

// validateEnabledFamilies requires a non-empty list of known, unique catalog names.
func validateEnabledFamilies(names []string) error {
	if len(names) == 0 {
		return errors.New("enabled_families must not be empty")
	}
	known := map[string]bool{}
	for _, n := range collector.AllFamilyNames() {
		known[n] = true
	}
	seen := map[string]bool{}
	for _, n := range names {
		if strings.TrimSpace(n) == "" {
			return errors.New("enabled_families contains empty name")
		}
		if !known[n] {
			return fmt.Errorf("unknown enabled_families entry %q", n)
		}
		if seen[n] {
			return fmt.Errorf("duplicate enabled_families entry %q", n)
		}
		seen[n] = true
	}
	return nil
}

// Interval returns the nominal 1 Hz frame period used by the Agent scheduler.
func Interval() time.Duration { return time.Second }

// RestartRequiredMessage is returned by config validate so operators know
// on-disk changes do not apply until the daemon is restarted.
const RestartRequiredMessage = "Config is valid. Restart infrasible-agent for changes to take effect."
