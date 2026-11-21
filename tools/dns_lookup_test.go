package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func execDNSLookup(t *testing.T, args map[string]any) (map[string]any, error) {
	t.Helper()
	r := &Registry{tools: make(map[string]registeredTool)}
	registerDNSLookup(r)
	b, _ := json.Marshal(args)
	out, err := r.Execute(context.Background(), "dns_lookup", b)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal result: %v\noutput: %s", err, out)
	}
	return m, nil
}

func TestDNSLookupSchema(t *testing.T) {
	r := &Registry{tools: make(map[string]registeredTool)}
	registerDNSLookup(r)
	defs := r.List()
	if len(defs) != 1 {
		t.Fatalf("expected 1 def, got %d", len(defs))
	}
	def := defs[0]
	if def.Function.Name != "dns_lookup" {
		t.Fatalf("unexpected name: %s", def.Function.Name)
	}
	params, ok := def.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("parameters not map")
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("required not []string: %T", params["required"])
	}
	hasHost, hasType := false, false
	for _, r := range required {
		if r == "host" {
			hasHost = true
		}
		if r == "type" {
			hasType = true
		}
	}
	if !hasHost || !hasType {
		t.Fatalf("required must include host and type: %#v", required)
	}
	props := params["properties"].(map[string]any)
	typeProp := props["type"].(map[string]any)
	enum := typeProp["enum"].([]string)
	want := map[string]bool{"A": false, "AAAA": false, "CNAME": false, "MX": false, "NS": false, "TXT": false, "PTR": false, "ALL": false}
	for _, e := range enum {
		if _, ok := want[e]; ok {
			want[e] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Fatalf("missing enum value %s", k)
		}
	}
}

func TestDNSLookupRequiresType(t *testing.T) {
	r := &Registry{tools: make(map[string]registeredTool)}
	registerDNSLookup(r)
	_, err := r.Execute(context.Background(), "dns_lookup", json.RawMessage(`{"host":"example.com"}`))
	if err == nil {
		t.Fatalf("expected error when type missing")
	}
	if !strings.Contains(err.Error(), "type is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDNSLookupPTRRequiresIP(t *testing.T) {
	res, err := execDNSLookup(t, map[string]any{"host": "example.com", "type": "PTR"})
	if err != nil {
		t.Fatalf("unexpected exec error: %v", err)
	}
	errs, _ := res["errors"].([]any)
	if len(errs) == 0 {
		t.Fatalf("expected PTR error for hostname, got: %#v", res)
	}
	found := false
	for _, e := range errs {
		if strings.Contains(fmt.Sprint(e), "PTR lookup requires a valid IP address") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PTR IP validation error, got: %#v", errs)
	}
}

func TestDNSLookupDispatchAndStructure(t *testing.T) {
	// Real DNS lookups; skip individual type on network/lookup failure but verify structure when successful.
	tests := []struct {
		name       string
		host       string
		recType    string
		recordKey  string
		validate   func(t *testing.T, val any)
	}{
		{"A", "example.com", "A", "A", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				t.Fatalf("A records not non-empty array: %#v", val)
			}
		}},
		{"AAAA", "google.com", "AAAA", "AAAA", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok {
				t.Fatalf("AAAA records not array: %#v", val)
			}
			_ = arr // may be empty in some envs, but key should exist
		}},
		{"MX", "google.com", "MX", "MX", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				t.Fatalf("MX records not non-empty array: %#v", val)
			}
			first, ok := arr[0].(map[string]any)
			if !ok {
				t.Fatalf("MX record not object: %#v", arr[0])
			}
			if _, ok := first["host"]; !ok {
				t.Fatalf("MX missing host: %#v", first)
			}
			if _, ok := first["preference"]; !ok {
				t.Fatalf("MX missing preference: %#v", first)
			}
		}},
		{"NS", "example.com", "NS", "NS", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				t.Fatalf("NS records not non-empty array: %#v", val)
			}
		}},
		{"TXT", "example.com", "TXT", "TXT", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok {
				t.Fatalf("TXT records not array: %#v", val)
			}
			_ = arr
		}},
		{"CNAME", "www.github.com", "CNAME", "CNAME", func(t *testing.T, val any) {
			s, ok := val.(string)
			if !ok || strings.TrimSpace(s) == "" {
				t.Fatalf("CNAME not non-empty string: %#v", val)
			}
		}},
		{"PTR", "8.8.8.8", "PTR", "PTR", func(t *testing.T, val any) {
			arr, ok := val.([]any)
			if !ok || len(arr) == 0 {
				t.Fatalf("PTR records not non-empty array: %#v", val)
			}
		}},
		{"ALL", "example.com", "ALL", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := execDNSLookup(t, map[string]any{"host": tt.host, "type": tt.recType})
			if err != nil {
				t.Fatalf("exec error: %v", err)
			}
			if res["type"] != tt.recType {
				t.Fatalf("type mismatch: %#v", res["type"])
			}
			records, ok := res["records"].(map[string]any)
			if !ok {
				t.Fatalf("records missing or wrong type: %#v", res)
			}
			if tt.recType == "ALL" {
				// ALL should attempt multiple record keys; at least some should be present.
				if len(records) == 0 {
					errs, _ := res["errors"].([]any)
					t.Fatalf("ALL returned no records, errors=%#v", errs)
				}
				return
			}
			val, ok := records[tt.recordKey]
			if !ok {
				errs, _ := res["errors"].([]any)
				t.Skipf("record key %s missing (possibly network/env): errors=%#v", tt.recordKey, errs)
			}
			if tt.validate != nil {
				tt.validate(t, val)
			}
		})
	}
}
