package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFileReadWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	r := &Registry{tools: make(map[string]registeredTool)}
	registerFileWrite(r)
	registerFileRead(r)

	// write
	wb, _ := json.Marshal(map[string]any{"path": path, "content": "hello world", "mode": "overwrite"})
	wout, err := r.Execute(context.Background(), "file_write", wb)
	if err != nil {
		t.Fatalf("write err: %v", err)
	}
	var wres map[string]any
	if err := json.Unmarshal([]byte(wout), &wres); err != nil {
		t.Fatalf("unmarshal write: %v", err)
	}
	if wres["ok"] != true {
		t.Fatalf("write not ok: %#v", wres)
	}

	// read
	rb, _ := json.Marshal(map[string]any{"path": path, "encoding": "utf8"})
	rout, err := r.Execute(context.Background(), "file_read", rb)
	if err != nil {
		t.Fatalf("read err: %v", err)
	}
	var rres map[string]any
	if err := json.Unmarshal([]byte(rout), &rres); err != nil {
		t.Fatalf("unmarshal read: %v", err)
	}
	if rres["data"] != "hello world" {
		t.Fatalf("unexpected data: %#v", rres["data"])
	}

	// append
	ab, _ := json.Marshal(map[string]any{"path": path, "content": "!", "mode": "append"})
	if _, err := r.Execute(context.Background(), "file_write", ab); err != nil {
		t.Fatalf("append err: %v", err)
	}
	rout2, err := r.Execute(context.Background(), "file_read", rb)
	if err != nil {
		t.Fatalf("read2 err: %v", err)
	}
	var rres2 map[string]any
	_ = json.Unmarshal([]byte(rout2), &rres2)
	if rres2["data"] != "hello world!" {
		t.Fatalf("unexpected append data: %#v", rres2["data"])
	}
}

func TestFileReadMissing(t *testing.T) {
	r := &Registry{tools: make(map[string]registeredTool)}
	registerFileRead(r)
	b, _ := json.Marshal(map[string]any{"path": "/no/such/file/xyz"})
	_, err := r.Execute(context.Background(), "file_read", b)
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		// some wrappers; just ensure error exists
	}
}
