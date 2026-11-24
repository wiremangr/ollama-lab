package tools

import (
	"context"
	"encoding/json"
	"net"
	"testing"
)

func TestTCPClient(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
		_, _ = conn.Write([]byte("echo:" + string(buf[:n])))
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	r := &Registry{tools: make(map[string]registeredTool)}
	registerTCPClient(r)

	// connect
	cb, _ := json.Marshal(map[string]any{"action": "connect", "host": "127.0.0.1", "port": port})
	cout, err := r.Execute(context.Background(), "tcp_client", cb)
	if err != nil {
		t.Fatalf("connect err: %v", err)
	}
	var cres map[string]any
	_ = json.Unmarshal([]byte(cout), &cres)
	if cres["ok"] != true {
		t.Fatalf("connect not ok: %#v", cres)
	}
	sid, _ := cres["session_id"].(string)
	if sid == "" {
		t.Fatalf("missing session_id: %#v", cres)
	}

	// send
	sb, _ := json.Marshal(map[string]any{"action": "send", "session_id": sid, "data": "ping"})
	sout, err := r.Execute(context.Background(), "tcp_client", sb)
	if err != nil {
		t.Fatalf("send err: %v", err)
	}
	var sres map[string]any
	_ = json.Unmarshal([]byte(sout), &sres)
	if sres["ok"] != true {
		t.Fatalf("send not ok: %#v", sres)
	}

	// recv
	rb, _ := json.Marshal(map[string]any{"action": "recv", "session_id": sid, "max_bytes": 1024, "timeout_seconds": 5})
	rout, err := r.Execute(context.Background(), "tcp_client", rb)
	if err != nil {
		t.Fatalf("recv err: %v", err)
	}
	var rres map[string]any
	_ = json.Unmarshal([]byte(rout), &rres)
	if rres["ok"] != true {
		t.Fatalf("recv not ok: %#v", rres)
	}
	if rres["data"] != "echo:ping" {
		t.Fatalf("unexpected recv data: %#v", rres["data"])
	}

	// close
	clb, _ := json.Marshal(map[string]any{"action": "close", "session_id": sid})
	clout, err := r.Execute(context.Background(), "tcp_client", clb)
	if err != nil {
		t.Fatalf("close err: %v", err)
	}
	var clres map[string]any
	_ = json.Unmarshal([]byte(clout), &clres)
	if clres["ok"] != true {
		t.Fatalf("close not ok: %#v", clres)
	}
}
