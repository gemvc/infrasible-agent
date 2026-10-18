package transport

import (
	"github.com/gemvc/infrasible-agent/internal/config"
	"testing"
)

func TestNoPlaintextEndpoint(t *testing.T) {
	c := config.Config{ConfigVersion: 1, MonitoringServers: []config.Server{{ID: "a", LiveURL: "ws://example.test", ServerName: "example.test", CAFile: "ca", ClientKeyFile: "key", ClientCertFile: "cert"}}}
	if c.Validate() == nil {
		t.Fatal("insecure WS accepted")
	}
}
