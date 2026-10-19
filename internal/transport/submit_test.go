package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gemvc/infrasible-agent/internal/collector"
	"github.com/gemvc/infrasible-agent/internal/config"
	"github.com/gemvc/infrasible-agent/internal/storage"
)

// writeMTLSMaterial creates a CA, server/client certs, and returns client paths
// plus a TLS config for an httptest server that requires client certs.
func writeMTLSMaterial(t *testing.T, dir string) (caPath, certPath, keyPath string, serverTLS *tls.Config) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(cn string, serial int64, usage x509.ExtKeyUsage) (certPEM, keyPEM []byte) {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(24 * time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			DNSNames:     []string{"localhost"},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyBytes, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	}
	caPath = filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	clientCert, clientKey := leaf("client", 2, x509.ExtKeyUsageClientAuth)
	certPath = filepath.Join(dir, "client.pem")
	keyPath = filepath.Join(dir, "client.key")
	if err := os.WriteFile(certPath, clientCert, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, clientKey, 0o600); err != nil {
		t.Fatal(err)
	}
	serverCert, serverKey := leaf("localhost", 3, x509.ExtKeyUsageServerAuth)
	pair, err := tls.X509KeyPair(serverCert, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	serverTLS = &tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	return caPath, certPath, keyPath, serverTLS
}

func seedPending(t *testing.T, dbPath string) (*storage.Store, storage.Pending) {
	t.Helper()
	s, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	begin := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 15; i++ {
		at := begin.Add(time.Duration(i) * time.Minute)
		f := collector.Frame{
			ObservedAtMS: at.UnixMilli(),
			ElapsedMS:    1000,
			Metrics: map[string]collector.Observation{
				"cpu": {Status: "valid", MeasuredAtMS: at.UnixMilli(), Unit: "percent", Value: map[string]float64{"total_pct": float64(i)}},
			},
		}
		if err := s.Save(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Finalize(ctx, begin.Add(18*time.Minute)); err != nil {
		t.Fatal(err)
	}
	p, ok, err := s.OldestPending(ctx)
	if err != nil || !ok {
		t.Fatalf("pending: ok=%v err=%v", ok, err)
	}
	return s, p
}

func TestLiveReturnsWhenNoCandidates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Sender{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Live(ctx, make(chan collector.Frame))
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Live did not return for empty servers")
	}
}

func TestPersistenceReturnsWhenNoEndpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		(&Sender{Log: slog.New(slog.NewTextHandler(io.Discard, nil))}).Persistence(ctx)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Persistence did not return for empty servers")
	}
}

func TestSubmitAcknowledgesOnlyValidACK(t *testing.T) {
	dir := t.TempDir()
	caPath, certPath, keyPath, serverTLS := writeMTLSMaterial(t, dir)
	store, pending := seedPending(t, filepath.Join(dir, "agent.db"))
	defer store.Close()

	var ackBody ACK
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ackBody)
	}))
	srv.TLS = serverTLS
	srv.StartTLS()
	defer srv.Close()

	server := config.Server{
		ID: "primary", PersistenceURL: srv.URL, PersistenceDomain: "domain-a",
		ServerName: "localhost", CAFile: caPath, ClientCertFile: certPath, ClientKeyFile: keyPath,
	}
	sender := &Sender{Store: store, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	ackBody = ACK{
		Durable: true, AgentID: "wrong", RecordID: pending.RecordID, PayloadSHA256: pending.Hash,
		PeriodStartMS: pending.PeriodStartMS, PersistenceDomain: "domain-a", SchemaVersion: 2,
	}
	if err := sender.submit(context.Background(), server, pending); err == nil {
		t.Fatal("invalid ACK accepted")
	}
	still, ok, err := store.OldestPending(context.Background())
	if err != nil || !ok || still.RecordID != pending.RecordID {
		t.Fatalf("pending cleared after bad ACK: ok=%v err=%v", ok, err)
	}

	ackBody = ACK{
		Durable: true, AgentID: store.AgentID(), RecordID: pending.RecordID, PayloadSHA256: pending.Hash,
		PeriodStartMS: pending.PeriodStartMS, PersistenceDomain: "domain-a", SchemaVersion: 2,
	}
	if err := sender.submit(context.Background(), server, pending); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.OldestPending(context.Background()); err != nil || ok {
		t.Fatalf("pending remained after valid ACK: ok=%v err=%v", ok, err)
	}
}
