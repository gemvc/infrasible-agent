package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
)

// Contract fixtures lock the executable Live wire after Hello/heartbeat:
// agent_hello → hello_accepted, then type=live frames and type=heartbeat.
// See docs/design/AGENT_PROTOCOL_AND_SECURITY.md §12 and AGENT_LIVE_HELLO_HEARTBEAT.md.
// Mock Bridge / Persistence doubles only — not production services.

// liveEnvelope is the JSON shape the Agent writes for Live sample frames.
type liveEnvelope struct {
	Type    string          `json:"type"`
	AgentID string          `json:"agent_id"`
	Frame   collector.Frame `json:"frame"`
}

type wireMessage struct {
	Type              string `json:"type"`
	AgentID           string `json:"agent_id"`
	ProtocolMajor     int    `json:"protocol_major"`
	AuthorizedAgentID string `json:"authorized_agent_id"`
	AuthorizedChannel string `json:"authorized_channel"`
	HeartbeatSeconds  int    `json:"heartbeat_seconds"`
	SentAtUTC         string `json:"sent_at_utc"`
}

// finalEnvelope is the Persistence POST body shape from storage.Final.
type finalEnvelope struct {
	AgentID           string `json:"agent_id"`
	SchemaVersion     int    `json:"schema_version"`
	PeriodStartMS     int64  `json:"period_start_ms"`
	ResolutionSeconds int    `json:"resolution_seconds"`
	RecordID          string `json:"record_id"`
	PayloadSHA256     string `json:"payload_sha256"`
}

// validateFinalContract rejects anything other than an immutable 900s final.
// Mirrors the central API rule: raw and 60s aggregates must not be accepted.
func validateFinalContract(body []byte) error {
	var fin finalEnvelope
	if err := json.Unmarshal(body, &fin); err != nil {
		return err
	}
	if fin.SchemaVersion != 2 {
		return errContract("unsupported schema_version")
	}
	if fin.ResolutionSeconds != 900 {
		return errContract("only resolution_seconds=900 finals accepted")
	}
	if fin.AgentID == "" || fin.RecordID == "" || fin.PayloadSHA256 == "" || fin.PeriodStartMS < 0 {
		return errContract("incomplete final identity")
	}
	return nil
}

type contractError string

func (e contractError) Error() string { return string(e) }

func errContract(msg string) error { return contractError(msg) }

func TestMockPersistenceRejectsRawAndMinutePayloads(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"raw-like", `{"agent_id":"a","schema_version":2,"resolution_seconds":1,"record_id":"r","payload_sha256":"h","period_start_ms":0}`},
		{"minute", `{"agent_id":"a","schema_version":2,"resolution_seconds":60,"record_id":"r","payload_sha256":"h","period_start_ms":0}`},
		{"bad schema", `{"agent_id":"a","schema_version":1,"resolution_seconds":900,"record_id":"r","payload_sha256":"h","period_start_ms":0}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateFinalContract([]byte(tc.body)); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
	ok := `{"agent_id":"a","schema_version":2,"resolution_seconds":900,"record_id":"r","payload_sha256":"h","period_start_ms":0}`
	if err := validateFinalContract([]byte(ok)); err != nil {
		t.Fatal(err)
	}
}

func TestMockPersistenceHTTP200WithoutACKLeavesPending(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, pending := seedPending(t, filepath.Join(dir, "agent.db"))
	defer store.Close()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	server := config.Server{
		ID: "primary", PersistenceURL: srv.URL, PersistenceDomain: "domain-a",
		ServerName: "localhost", CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
	}
	sender := &Sender{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := sender.submit(context.Background(), server, pending); err == nil {
		t.Fatal("empty ACK must fail submit")
	}
	still, ok, err := store.OldestPending(context.Background())
	if err != nil || !ok || still.RecordID != pending.RecordID || !bytes.Equal(still.Payload, pending.Payload) {
		t.Fatalf("pending mutated after bare HTTP 200: ok=%v err=%v", ok, err)
	}
}

func TestMockPersistenceLostACKThenIdenticalReplay(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, pending := seedPending(t, filepath.Join(dir, "agent.db"))
	defer store.Close()

	var (
		mu        sync.Mutex
		attempts  int
		bodies    [][]byte
		committed map[string]string // record_id -> hash (central dedup)
	)
	committed = map[string]string{}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		if err := validateFinalContract(body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Header.Get("Idempotency-Key") == "" {
			http.Error(w, "missing Idempotency-Key", http.StatusBadRequest)
			return
		}
		var fin finalEnvelope
		_ = json.Unmarshal(body, &fin)

		mu.Lock()
		attempts++
		n := attempts
		bodies = append(bodies, append([]byte(nil), body...))
		if prev, ok := committed[fin.RecordID]; ok && prev != fin.PayloadSHA256 {
			mu.Unlock()
			http.Error(w, "hash conflict", http.StatusConflict)
			return
		}
		committed[fin.RecordID] = fin.PayloadSHA256
		mu.Unlock()

		// First attempt: durable commit succeeds but ACK is lost (no body).
		if n == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		// Replay: identical bytes yield one logical row and a valid ACK.
		_ = json.NewEncoder(w).Encode(ACK{
			Durable: true, AgentID: fin.AgentID, RecordID: fin.RecordID,
			PayloadSHA256: fin.PayloadSHA256, PeriodStartMS: fin.PeriodStartMS,
			PersistenceDomain: "domain-a", SchemaVersion: 2,
		})
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	server := config.Server{
		ID: "primary", PersistenceURL: srv.URL, PersistenceDomain: "domain-a",
		ServerName: "localhost", CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
	}
	sender := &Sender{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	if err := sender.submit(context.Background(), server, pending); err == nil {
		t.Fatal("lost-ACK response must not acknowledge locally")
	}
	again, ok, err := store.OldestPending(context.Background())
	if err != nil || !ok {
		t.Fatal("pending cleared after lost ACK")
	}
	if !bytes.Equal(again.Payload, pending.Payload) || again.Hash != pending.Hash || again.RecordID != pending.RecordID {
		t.Fatal("replay payload must be byte-identical to first attempt")
	}
	if err := sender.submit(context.Background(), server, again); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.OldestPending(context.Background()); err != nil || ok {
		t.Fatalf("pending remained after successful replay ACK: ok=%v err=%v", ok, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if attempts != 2 || len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatalf("central must see identical replay bytes: attempts=%d bodies=%d", attempts, len(bodies))
	}
	if len(committed) != 1 {
		t.Fatalf("central dedup must keep one logical row, got %d", len(committed))
	}
}

func TestAgentWirePayloadIs900sFinalOnly(t *testing.T) {
	store, pending := seedPending(t, filepath.Join(t.TempDir(), "agent.db"))
	defer store.Close()
	if err := validateFinalContract(pending.Payload); err != nil {
		t.Fatal(err)
	}
	var fin finalEnvelope
	if err := json.Unmarshal(pending.Payload, &fin); err != nil {
		t.Fatal(err)
	}
	if fin.ResolutionSeconds != 900 || fin.SchemaVersion != 2 {
		t.Fatalf("unexpected final: %+v", fin)
	}
	if fin.PayloadSHA256 != pending.Hash || fin.RecordID != pending.RecordID {
		t.Fatal("wire identity mismatch vs pending metadata")
	}
}

func TestValidateHelloAcceptedBounds(t *testing.T) {
	agentID := "agent-1"
	ok := []byte(`{"type":"hello_accepted","protocol_major":2,"protocol_minor":0,"schema_version":"2.0","authorized_agent_id":"agent-1","authorized_channel":"live","endpoint_id":"monitor-a","heartbeat_seconds":15}`)
	d, err := validateHelloAccepted(ok, agentID)
	if err != nil || d != 15*time.Second {
		t.Fatalf("got %v %v", d, err)
	}
	cases := []struct {
		name string
		body string
	}{
		{"wrong type", `{"type":"live","protocol_major":2,"authorized_agent_id":"agent-1","authorized_channel":"live","heartbeat_seconds":15}`},
		{"wrong major", `{"type":"hello_accepted","protocol_major":1,"authorized_agent_id":"agent-1","authorized_channel":"live","heartbeat_seconds":15}`},
		{"wrong agent", `{"type":"hello_accepted","protocol_major":2,"authorized_agent_id":"other","authorized_channel":"live","heartbeat_seconds":15}`},
		{"wrong channel", `{"type":"hello_accepted","protocol_major":2,"authorized_agent_id":"agent-1","authorized_channel":"persistence","heartbeat_seconds":15}`},
		{"zero heartbeat", `{"type":"hello_accepted","protocol_major":2,"authorized_agent_id":"agent-1","authorized_channel":"live","heartbeat_seconds":0}`},
		{"huge heartbeat", `{"type":"hello_accepted","protocol_major":2,"authorized_agent_id":"agent-1","authorized_channel":"live","heartbeat_seconds":301}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateHelloAccepted([]byte(tc.body), agentID); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

// writeHelloAccepted is a transient mock Bridge helper (RAM only; no history store).
func writeHelloAccepted(conn *websocket.Conn, ctx context.Context, agentID string, heartbeatSec int) error {
	payload, err := json.Marshal(helloAccepted{
		Type: "hello_accepted", ProtocolMajor: 2, ProtocolMinor: 0, SchemaVersion: "2.0",
		AuthorizedAgentID: agentID, AuthorizedChannel: "live", EndpointID: "monitor-a",
		HeartbeatSeconds: heartbeatSec, MaxMessageBytes: 262144,
	})
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, payload)
}

func TestMockBridgeAcceptsLiveEnvelopeAndStoresNoHistory(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var (
		mu       sync.Mutex
		received []wireMessage
		history  []wireMessage // intentionally unused — Bridge must not expose history
	)
	_ = history

	gotHello := make(chan wireMessage, 1)
	gotFrame := make(chan liveEnvelope, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()

		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var first wireMessage
		if err := json.Unmarshal(data, &first); err != nil {
			return
		}
		mu.Lock()
		received = append(received, first)
		mu.Unlock()
		select {
		case gotHello <- first:
		default:
		}
		if first.Type != "agent_hello" {
			return
		}
		if err := writeHelloAccepted(conn, ctx, store.AgentID(), 15); err != nil {
			return
		}
		// Keep peer non-silent while waiting for the live frame.
		go func() {
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"heartbeat","sent_at_utc":"2026-10-18T12:00:00Z"}`))
				}
			}
		}()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var env liveEnvelope
			if err := json.Unmarshal(data, &env); err != nil {
				continue
			}
			mu.Lock()
			received = append(received, wireMessage{Type: env.Type, AgentID: env.AgentID})
			mu.Unlock()
			if env.Type == "live" {
				select {
				case gotFrame <- env:
				default:
				}
				return
			}
		}
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	liveURL := "wss://" + strings.TrimPrefix(srv.URL, "https://")
	frames := make(chan collector.Frame, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &Sender{
		Servers: []config.Server{{
			ID: "bridge", LiveURL: liveURL, ServerName: "localhost",
			CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
		}},
		Store:        store,
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		AgentVersion: "0.3.5",
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Live(ctx, frames)
	}()

	var hello wireMessage
	select {
	case hello = <-gotHello:
	case <-time.After(5 * time.Second):
		t.Fatal("mock Bridge did not receive agent_hello first")
	}
	if hello.Type != "agent_hello" || hello.ProtocolMajor != 2 || hello.AgentID != store.AgentID() {
		t.Fatalf("unexpected hello: %+v", hello)
	}

	frame := collector.Frame{
		ObservedAtMS: time.Date(2026, 10, 18, 12, 0, 1, 0, time.UTC).UnixMilli(),
		ElapsedMS:    1000,
		Metrics: map[string]collector.Observation{
			"cpu": {Status: "valid", MeasuredAtMS: time.Date(2026, 10, 18, 12, 0, 1, 0, time.UTC).UnixMilli(), Unit: "percent", Value: map[string]float64{"total_pct": 12.5}},
		},
	}
	select {
	case frames <- frame:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked sending live frame")
	}

	var env liveEnvelope
	select {
	case env = <-gotFrame:
	case <-time.After(5 * time.Second):
		t.Fatal("mock Bridge did not receive a live frame after hello")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Live worker did not exit")
	}

	if env.Type != "live" {
		t.Fatalf("expected type=live after hello, got %q", env.Type)
	}
	if env.AgentID != store.AgentID() {
		t.Fatalf("agent_id mismatch: %q vs %q", env.AgentID, store.AgentID())
	}
	if env.Frame.ObservedAtMS != frame.ObservedAtMS || env.Frame.ElapsedMS != frame.ElapsedMS {
		t.Fatalf("frame identity mismatch: %+v", env.Frame)
	}
	if _, ok := env.Frame.Metrics["cpu"]; !ok {
		t.Fatal("expected cpu metric in live frame")
	}
	if _, ok := env.Frame.Metrics["load"]; ok {
		t.Fatal("not-due load must not appear as a fabricated live metric in this fixture")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) < 2 {
		t.Fatalf("expected hello then live; got %d messages", len(received))
	}
	if received[0].Type != "agent_hello" {
		t.Fatalf("first message must be agent_hello, got %q", received[0].Type)
	}
}

func TestMockBridgeRejectsWrongAuthorizedAgentID(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prevTimeout := liveHelloTimeout
	liveHelloTimeout = 500 * time.Millisecond
	defer func() { liveHelloTimeout = prevTimeout }()

	hellos := make(chan struct{}, 4)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var first wireMessage
		if err := json.Unmarshal(data, &first); err != nil || first.Type != "agent_hello" {
			return
		}
		hellos <- struct{}{}
		payload, _ := json.Marshal(helloAccepted{
			Type: "hello_accepted", ProtocolMajor: 2, ProtocolMinor: 0, SchemaVersion: "2.0",
			AuthorizedAgentID: "not-this-agent", AuthorizedChannel: "live",
			EndpointID: "monitor-a", HeartbeatSeconds: 15,
		})
		_ = conn.Write(ctx, websocket.MessageText, payload)
		time.Sleep(100 * time.Millisecond)
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	liveURL := "wss://" + strings.TrimPrefix(srv.URL, "https://")
	frames := make(chan collector.Frame)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &Sender{
		Servers: []config.Server{{
			ID: "bridge", LiveURL: liveURL, ServerName: "localhost",
			CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
		}},
		Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), AgentVersion: "0.3.5",
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Live(ctx, frames)
	}()

	select {
	case <-hellos:
	case <-time.After(3 * time.Second):
		t.Fatal("expected first agent_hello")
	}
	// Wrong authorized_agent_id must fail negotiate and retry (second hello), never stream live.
	select {
	case <-hellos:
	case <-time.After(5 * time.Second):
		t.Fatal("expected reconnect after failed hello")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Live worker did not exit")
	}
}

func TestMockBridgeNoHelloReplyFailsNegotiate(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	prevTimeout := liveHelloTimeout
	liveHelloTimeout = 200 * time.Millisecond
	defer func() { liveHelloTimeout = prevTimeout }()

	attempts := make(chan struct{}, 4)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(r.Context(), time.Second)
		defer cancel()
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var first wireMessage
		if json.Unmarshal(data, &first) == nil && first.Type == "agent_hello" {
			attempts <- struct{}{}
		}
		// No hello_accepted — agent must fail negotiate and reconnect.
		time.Sleep(300 * time.Millisecond)
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	liveURL := "wss://" + strings.TrimPrefix(srv.URL, "https://")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &Sender{
		Servers: []config.Server{{
			ID: "bridge", LiveURL: liveURL, ServerName: "localhost",
			CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
		}},
		Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), AgentVersion: "0.3.5",
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Live(ctx, make(chan collector.Frame))
	}()

	select {
	case <-attempts:
	case <-time.After(2 * time.Second):
		t.Fatal("expected agent_hello")
	}
	select {
	case <-attempts:
	case <-time.After(5 * time.Second):
		t.Fatal("expected reconnect after hello timeout")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Live worker did not exit")
	}
}

func TestMockBridgeObservesHeartbeat(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, err := storage.Open(filepath.Join(dir, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	gotHeartbeat := make(chan wireMessage, 1)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "done")
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var first wireMessage
		if err := json.Unmarshal(data, &first); err != nil || first.Type != "agent_hello" {
			return
		}
		if err := writeHelloAccepted(conn, ctx, store.AgentID(), 1); err != nil {
			return
		}
		go func() {
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = conn.Write(ctx, websocket.MessageText, []byte(`{"type":"heartbeat","sent_at_utc":"2026-10-18T12:00:00Z"}`))
				}
			}
		}()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var msg wireMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}
			if msg.Type == "heartbeat" {
				select {
				case gotHeartbeat <- msg:
				default:
				}
				return
			}
		}
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	liveURL := "wss://" + strings.TrimPrefix(srv.URL, "https://")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sender := &Sender{
		Servers: []config.Server{{
			ID: "bridge", LiveURL: liveURL, ServerName: "localhost",
			CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
		}},
		Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), AgentVersion: "0.3.5",
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		sender.Live(ctx, make(chan collector.Frame))
	}()

	var hb wireMessage
	select {
	case hb = <-gotHeartbeat:
	case <-time.After(3 * time.Second):
		t.Fatal("expected application heartbeat within heartbeat_seconds+ε")
	}
	if hb.Type != "heartbeat" || hb.SentAtUTC == "" {
		t.Fatalf("unexpected heartbeat: %+v", hb)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Live worker did not exit")
	}
}

func TestMockPersistenceRejectsWrongDomainACK(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, pending := seedPending(t, filepath.Join(dir, "agent.db"))
	defer store.Close()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var fin finalEnvelope
		_ = json.Unmarshal(body, &fin)
		_ = json.NewEncoder(w).Encode(ACK{
			Durable: true, AgentID: fin.AgentID, RecordID: fin.RecordID,
			PayloadSHA256: fin.PayloadSHA256, PeriodStartMS: fin.PeriodStartMS,
			PersistenceDomain: "other-domain", SchemaVersion: 2,
		})
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	server := config.Server{
		ID: "primary", PersistenceURL: srv.URL, PersistenceDomain: "domain-a",
		ServerName: "localhost", CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
	}
	if err := (&Sender{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).
		submit(context.Background(), server, pending); err == nil {
		t.Fatal("cross-domain ACK must be rejected")
	}
	if _, ok, err := store.OldestPending(context.Background()); err != nil || !ok {
		t.Fatal("pending must survive wrong-domain ACK")
	}
}
