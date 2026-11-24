package tools

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

func makeTestTLSConfig(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("keypair: %v", err)
	}
	return cert
}

func TestTLSClient(t *testing.T) {
	cert := makeTestTLSConfig(t)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		_, _ = conn.Write([]byte("tls:" + string(buf[:n])))
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	r := &Registry{tools: make(map[string]registeredTool)}
	registerTLSClient(r)

	// connect without confirmation should require confirmation when insecure
	cb, _ := json.Marshal(map[string]any{"action": "connect", "host": "127.0.0.1", "port": port, "insecure_skip_verify": true})
	cout, err := r.Execute(context.Background(), "tls_client", cb)
	if err != nil {
		t.Fatalf("connect err: %v", err)
	}
	var cres map[string]any
	_ = json.Unmarshal([]byte(cout), &cres)
	if cres["status"] != "confirmation_required" {
		t.Fatalf("expected confirmation_required: %#v", cres)
	}

	// connect with confirmation
	cb2, _ := json.Marshal(map[string]any{"action": "connect", "host": "127.0.0.1", "port": port, "insecure_skip_verify": true, "confirm_insecure": true})
	cout2, err := r.Execute(context.Background(), "tls_client", cb2)
	if err != nil {
		t.Fatalf("connect2 err: %v", err)
	}
	var cres2 map[string]any
	_ = json.Unmarshal([]byte(cout2), &cres2)
	if cres2["ok"] != true {
		t.Fatalf("connect2 not ok: %#v", cres2)
	}
	sid, _ := cres2["session_id"].(string)

	// send/recv/close
	sb, _ := json.Marshal(map[string]any{"action": "send", "session_id": sid, "data": "hello"})
	if _, err := r.Execute(context.Background(), "tls_client", sb); err != nil {
		t.Fatalf("send err: %v", err)
	}
	rb, _ := json.Marshal(map[string]any{"action": "recv", "session_id": sid, "max_bytes": 1024, "timeout_seconds": 5})
	rout, err := r.Execute(context.Background(), "tls_client", rb)
	if err != nil {
		t.Fatalf("recv err: %v", err)
	}
	var rres map[string]any
	_ = json.Unmarshal([]byte(rout), &rres)
	if rres["data"] != "tls:hello" {
		t.Fatalf("unexpected data: %#v", rres["data"])
	}
	clb, _ := json.Marshal(map[string]any{"action": "close", "session_id": sid})
	if _, err := r.Execute(context.Background(), "tls_client", clb); err != nil {
		t.Fatalf("close err: %v", err)
	}
}
