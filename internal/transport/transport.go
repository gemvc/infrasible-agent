// This file is tested and fixed. Do not add, remove, or change the code in this file.
//
// Package transport implements outbound Live (WSS) and Persistence (HTTPS) mTLS clients.
package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
)

const (
	liveProtocolMajor = 2
	liveProtocolMinor = 0
	// maxLiveHeartbeatSeconds rejects negotiated intervals that would stall
	// fail-closed reconnect for an unreasonable time (documented in contract tests).
	maxLiveHeartbeatSeconds = 300
)

// liveHelloTimeout bounds waiting for hello_accepted after agent_hello.
// Tests may shorten this unexported variable.
var liveHelloTimeout = 10 * time.Second

// Sender delivers Live frames and Persistence finals using configured endpoints.
type Sender struct {
	Servers      []config.Server
	Store        *storage.Store
	Log          *slog.Logger
	AgentVersion string
}

// ACK is the durable Persistence acknowledgment required before local persisted_at.
type ACK struct {
	Durable           bool   `json:"durable"`
	AgentID           string `json:"agent_id"`
	RecordID          string `json:"record_id"`
	PayloadSHA256     string `json:"payload_sha256"`
	PeriodStartMS     int64  `json:"period_start_ms"`
	PersistenceDomain string `json:"persistence_domain"`
	SchemaVersion     int    `json:"schema_version"`
}

type agentHello struct {
	Type           string   `json:"type"`
	ProtocolMajor  int      `json:"protocol_major"`
	ProtocolMinor  int      `json:"protocol_minor"`
	AgentID        string   `json:"agent_id"`
	AgentVersion   string   `json:"agent_version"`
	Channel        string   `json:"channel"`
	SchemaVersions []string `json:"schema_versions"`
	Capabilities   []string `json:"capabilities"`
	SentAtUTC      string   `json:"sent_at_utc"`
}

type helloAccepted struct {
	Type              string `json:"type"`
	ProtocolMajor     int    `json:"protocol_major"`
	ProtocolMinor     int    `json:"protocol_minor"`
	SchemaVersion     string `json:"schema_version"`
	AuthorizedAgentID string `json:"authorized_agent_id"`
	AuthorizedChannel string `json:"authorized_channel"`
	EndpointID        string `json:"endpoint_id"`
	HeartbeatSeconds  int    `json:"heartbeat_seconds"`
	MaxMessageBytes   int    `json:"max_message_bytes"`
}

// tlsFor builds a TLS 1.3 client config from server identity files.
// The private key is opened with O_NOFOLLOW and must be owner-only.
func tlsFor(s config.Server) (*tls.Config, error) {
	// Open once with O_NOFOLLOW so key identity and permission validation
	// are tied to the same file descriptor used for reading the secret.
	keyFile, e := os.OpenFile(s.ClientKeyFile, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, fmt.Errorf("open client private key: %w", e)
	}
	defer keyFile.Close()
	keyInfo, e := keyFile.Stat()
	if e != nil {
		return nil, fmt.Errorf("stat client private key: %w", e)
	}
	if !keyInfo.Mode().IsRegular() || keyInfo.Mode().Perm()&0077 != 0 {
		return nil, errors.New("client private key must be a regular file accessible only to its owner")
	}
	// Reject unexpectedly large secrets and avoid reopening the path after
	// validating its mode (a classic pathname time-of-check/time-of-use race).
	keyPEM, e := io.ReadAll(io.LimitReader(keyFile, 1<<20+1))
	if e != nil {
		return nil, fmt.Errorf("read client private key: %w", e)
	}
	if len(keyPEM) > 1<<20 {
		return nil, errors.New("client private key exceeds 1 MiB")
	}
	pem, e := os.ReadFile(s.CAFile)
	if e != nil {
		return nil, e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid trusted CA")
	}
	certPEM, e := os.ReadFile(s.ClientCertFile)
	if e != nil {
		return nil, e
	}
	pair, e := tls.X509KeyPair(certPEM, keyPEM)
	if e != nil {
		return nil, e
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: s.ServerName, Certificates: []tls.Certificate{pair}}, nil
}

// Live reconnects through eligible LiveURL endpoints and streams frames without
// historical replay. An empty candidate list returns immediately.
func (s *Sender) Live(ctx context.Context, frames <-chan collector.Frame) {
	// Latest-only, bounded producer channel is owned by runtime; no historical replay.
	index := 0
	for {
		if ctx.Err() != nil {
			return
		}
		candidates := make([]config.Server, 0)
		for _, v := range s.Servers {
			if v.LiveURL != "" {
				candidates = append(candidates, v)
			}
		}
		if len(candidates) == 0 {
			return
		}
		server := candidates[index%len(candidates)]
		tlsConfig, e := tlsFor(server)
		if e != nil {
			s.Log.Warn("live credentials unavailable", "server", server.ID, "error", e)
		} else {
			client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}}
			conn, _, err := websocket.Dial(ctx, server.LiveURL, &websocket.DialOptions{HTTPClient: client})
			if err != nil {
				s.Log.Warn("live connect failed", "server", server.ID, "error", err)
			} else {
				s.Log.Info("live connected", "server", server.ID)
				heartbeat, err := s.negotiateLive(ctx, conn)
				if err != nil {
					s.Log.Warn("live hello failed", "server", server.ID, "error", err)
				} else {
					e = s.stream(ctx, conn, frames, heartbeat)
					if e != nil && ctx.Err() == nil {
						s.Log.Warn("live stream ended", "error", e)
					}
				}
				_ = conn.Close(websocket.StatusNormalClosure, "reconnecting")
			}
			client.CloseIdleConnections()
		}
		index++
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

// negotiateLive sends agent_hello and validates hello_accepted before any live frames.
func (s *Sender) negotiateLive(ctx context.Context, conn *websocket.Conn) (time.Duration, error) {
	version := s.AgentVersion
	if version == "" {
		return 0, errors.New("agent version required for live hello")
	}
	hello := agentHello{
		Type:           "agent_hello",
		ProtocolMajor:  liveProtocolMajor,
		ProtocolMinor:  liveProtocolMinor,
		AgentID:        s.Store.AgentID(),
		AgentVersion:   version,
		Channel:        "live",
		SchemaVersions: []string{"2.0"},
		Capabilities:   []string{"live_1hz", "sample_quality", "metric_cadence_v1", "metric_freshness_v1"},
		SentAtUTC:      time.Now().UTC().Format(time.RFC3339),
	}
	payload, err := json.Marshal(hello)
	if err != nil {
		return 0, err
	}
	writeCtx, cancelWrite := context.WithTimeout(ctx, 5*time.Second)
	err = conn.Write(writeCtx, websocket.MessageText, payload)
	cancelWrite()
	if err != nil {
		return 0, err
	}
	readCtx, cancelRead := context.WithTimeout(ctx, liveHelloTimeout)
	_, data, err := conn.Read(readCtx)
	cancelRead()
	if err != nil {
		return 0, fmt.Errorf("hello_accepted: %w", err)
	}
	return validateHelloAccepted(data, s.Store.AgentID())
}

// validateHelloAccepted fail-closes on type, protocol, identity, channel, or heartbeat bounds.
// max_message_bytes is parsed when present but not enforced on outbound writes in this slice.
func validateHelloAccepted(data []byte, agentID string) (time.Duration, error) {
	var accepted helloAccepted
	if err := json.Unmarshal(data, &accepted); err != nil {
		return 0, err
	}
	if accepted.Type != "hello_accepted" {
		return 0, fmt.Errorf("expected hello_accepted, got %q", accepted.Type)
	}
	if accepted.ProtocolMajor != liveProtocolMajor {
		return 0, fmt.Errorf("unsupported protocol_major %d", accepted.ProtocolMajor)
	}
	if accepted.AuthorizedAgentID != agentID {
		return 0, errors.New("authorized_agent_id mismatch")
	}
	if accepted.AuthorizedChannel != "live" {
		return 0, fmt.Errorf("authorized_channel %q not live", accepted.AuthorizedChannel)
	}
	if accepted.HeartbeatSeconds <= 0 || accepted.HeartbeatSeconds > maxLiveHeartbeatSeconds {
		return 0, fmt.Errorf("heartbeat_seconds out of range: %d", accepted.HeartbeatSeconds)
	}
	return time.Duration(accepted.HeartbeatSeconds) * time.Second, nil
}

// stream writes live JSON envelopes and application heartbeats until the context
// ends, the channel closes, or the peer is silent for ~3× the heartbeat interval.
func (s *Sender) stream(ctx context.Context, conn *websocket.Conn, frames <-chan collector.Frame, heartbeat time.Duration) error {
	type live struct {
		Type    string          `json:"type"`
		AgentID string          `json:"agent_id"`
		Frame   collector.Frame `json:"frame"`
	}
	type heartbeatMsg struct {
		Type      string `json:"type"`
		SentAtUTC string `json:"sent_at_utc"`
	}

	var lastPeerUnix atomic.Int64
	lastPeerUnix.Store(time.Now().UnixNano())
	silence := 3 * heartbeat

	readErr := make(chan error, 1)
	go func() {
		for {
			readCtx, cancel := context.WithTimeout(ctx, silence+time.Second)
			_, _, err := conn.Read(readCtx)
			cancel()
			if err != nil {
				readErr <- err
				return
			}
			lastPeerUnix.Store(time.Now().UnixNano())
		}
	}()

	ticker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	silenceTicker := time.NewTicker(heartbeat)
	defer silenceTicker.Stop()

	writeText := func(payload []byte) error {
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := conn.Write(sendCtx, websocket.MessageText, payload)
		cancel()
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErr:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("live peer read: %w", err)
		case <-silenceTicker.C:
			last := time.Unix(0, lastPeerUnix.Load())
			if time.Since(last) > silence {
				return errors.New("live peer silent beyond 3x heartbeat")
			}
		case <-ticker.C:
			payload, err := json.Marshal(heartbeatMsg{
				Type:      "heartbeat",
				SentAtUTC: time.Now().UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			if err := writeText(payload); err != nil {
				return err
			}
		case f, ok := <-frames:
			if !ok {
				return nil
			}
			payload, err := json.Marshal(live{Type: "live", AgentID: s.Store.AgentID(), Frame: f})
			if err != nil {
				return err
			}
			if err := writeText(payload); err != nil {
				return err
			}
		}
	}
}

// Persistence submits oldest pending 900s finals to the first configured
// PersistenceURL. No endpoint returns immediately.
func (s *Sender) Persistence(ctx context.Context) {
	var selected *config.Server
	// No cross-endpoint assumption about shared durable domains is made before
	// external issuer/domain attestations are implemented and accepted.
	for i := range s.Servers {
		if s.Servers[i].PersistenceURL != "" {
			selected = &s.Servers[i]
			break
		}
	}
	if selected == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		p, ok, e := s.Store.OldestPending(ctx)
		if e != nil {
			s.Log.Error("pending read", "error", e)
		} else if ok {
			if e = s.submit(ctx, *selected, p); e != nil {
				s.Log.Warn("persistence retry", "server", selected.ID, "error", e)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// submit POSTs a pending final over mTLS and Acknowledge only after validateACK.
func (s *Sender) submit(ctx context.Context, server config.Server, p storage.Pending) error {
	tlsConfig, e := tlsFor(server)
	if e != nil {
		return e
	}
	transport := &http.Transport{TLSClientConfig: tlsConfig, Proxy: http.ProxyFromEnvironment}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 20 * time.Second}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, server.PersistenceURL, bytes.NewReader(p.Payload))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", p.RecordID)
	resp, e := client.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("durable API HTTP %d", resp.StatusCode)
	}
	body := io.LimitReader(resp.Body, 64*1024)
	var ack ACK
	if e = json.NewDecoder(body).Decode(&ack); e != nil {
		return e
	}
	if e = validateACK(ack, p, s.Store.AgentID(), server.PersistenceDomain); e != nil {
		return e
	}
	return s.Store.Acknowledge(ctx, p.RecordID, p.Hash, time.Now().UTC())
}

// validateACK requires every durable ACK field to match the pending final.
func validateACK(ack ACK, p storage.Pending, agentID, domain string) error {
	if !ack.Durable || ack.AgentID != agentID || ack.RecordID != p.RecordID || ack.PayloadSHA256 != p.Hash ||
		ack.PeriodStartMS != p.PeriodStartMS || ack.PersistenceDomain != domain || ack.SchemaVersion != 2 {
		return errors.New("invalid/unmatched durable ACK")
	}
	return nil
}
