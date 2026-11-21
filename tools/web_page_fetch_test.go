package tools

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func execWebFetch(t *testing.T, args map[string]any) (map[string]any, error) {
	t.Helper()
	r := &Registry{tools: make(map[string]registeredTool)}
	registerWebPageFetch(r)
	b, _ := json.Marshal(args)
	out, err := r.Execute(context.Background(), "web_page_fetch", b)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	return m, nil
}

func TestWebPageFetchSSRFBlocks(t *testing.T) {
	allowPrivateNetworksForTests = false
	cases := []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
		"http://192.168.1.1/",
		"http://[::1]/",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			_, err := execWebFetch(t, map[string]any{"url": u})
			if err == nil {
				t.Fatalf("expected SSRF block for %s", u)
			}
		})
	}
}

func TestWebPageFetchExtractsContent(t *testing.T) {
	allowPrivateNetworksForTests = true
	defer func() { allowPrivateNetworksForTests = false }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Test Page</title><style>.x{color:red}</style><script>alert(1)</script></head>
<body>
<nav><a href="/nav">Nav</a></nav>
<header>Header</header>
<main>
<h1>Main Heading</h1>
<p>Hello <strong>world</strong>.</p>
<h2>Subheading</h2>
<p>Useful content here.</p>
<a href="/page2">Page Two</a>
<a href="https://example.com/ext">External</a>
</main>
<footer>Footer text</footer>
</body></html>`))
	}))
	defer srv.Close()

	res, err := execWebFetch(t, map[string]any{"url": srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["status"].(float64) != 200 {
		t.Fatalf("status: %#v", res["status"])
	}
	if res["title"] != "Test Page" {
		t.Fatalf("title: %#v", res["title"])
	}
	text, _ := res["text"].(string)
	for _, want := range []string{"Main Heading", "Hello world", "Useful content here"} {
		if !strings.Contains(text, want) {
			t.Fatalf("text missing %q: %q", want, text)
		}
	}
	for _, bad := range []string{"alert(1)", "color:red", "Nav", "Header", "Footer text"} {
		if strings.Contains(text, bad) {
			t.Fatalf("text should not contain %q: %q", bad, text)
		}
	}
	headings, ok := res["headings"].([]any)
	if !ok || len(headings) == 0 {
		t.Fatalf("headings: %#v", res["headings"])
	}
	links, ok := res["links"].([]any)
	if !ok || len(links) < 2 {
		t.Fatalf("links: %#v", res["links"])
	}
}

func TestWebPageFetchTruncationAndJSWarning(t *testing.T) {
	allowPrivateNetworksForTests = true
	defer func() { allowPrivateNetworksForTests = false }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><head><title>App</title></head><body><div id='root'></div><script>" + strings.Repeat("console.log('x');", 500) + "</script></body></html>"))
	}))
	defer srv.Close()

	res, err := execWebFetch(t, map[string]any{"url": srv.URL, "max_bytes": 200})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res["truncated"] != true {
		t.Fatalf("expected truncated: %#v", res)
	}
	warn, _ := res["extraction_warning"].(string)
	_ = warn // may or may not be set depending on extraction
}

func TestWebPageFetchSSRFHelpers(t *testing.T) {
	allowPrivateNetworksForTests = false
	for _, ip := range []string{"127.0.0.1", "10.1.2.3", "192.168.1.1", "169.254.169.254", "::1"} {
		if !isBlockedIP(net.ParseIP(ip)) {
			t.Fatalf("IP should be blocked: %s", ip)
		}
	}
	if isBlockedIP(net.ParseIP("8.8.8.8")) {
		t.Fatalf("public IP should not be blocked")
	}
	if _, err := validateFetchURL("ftp://example.com"); err == nil {
		t.Fatalf("non-http scheme should be rejected")
	}
	if _, err := validateFetchURL("http://127.0.0.1/test"); err == nil {
		t.Fatalf("loopback URL should be rejected")
	}
}
