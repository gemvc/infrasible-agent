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
	"syscall"
	"time"

	"github.com/coder/websocket"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
)

// Sender delivers Live frames and Persistence finals using configured endpoints.
type Sender struct {
	Servers []config.Server
	Store   *storage.Store
	Log     *slog.Logger
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
				e = s.stream(ctx, conn, frames)
				_ = conn.Close(websocket.StatusNormalClosure, "reconnecting")
				if e != nil && ctx.Err() == nil {
					s.Log.Warn("live stream ended", "error", e)
				}
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

// stream writes live JSON envelopes until the context ends or the channel closes.
func (s *Sender) stream(ctx context.Context, conn *websocket.Conn, frames <-chan collector.Frame) error {
	type live struct {
		Type    string          `json:"type"`
		AgentID string          `json:"agent_id"`
		Frame   collector.Frame `json:"frame"`
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case f, ok := <-frames:
			if !ok {
				return nil
			}
			payload, e := json.Marshal(live{Type: "live", AgentID: s.Store.AgentID(), Frame: f})
			if e != nil {
				return e
			}
			sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			e = conn.Write(sendCtx, websocket.MessageText, payload)
			cancel()
			if e != nil {
				return e
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
