package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

func registerWebPageFetch(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name: "web_page_fetch",
			Description: "Fetch a web page and return structured extracted content. " +
				"Returns URL, status, title, headings, extracted readable text, useful links, and truncation/extraction metadata. " +
				"Blocks private/loopback/metadata IP targets and validates redirects for SSRF protection.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url": map[string]any{"type": "string", "description": "HTTP or HTTPS URL to fetch"},
					"max_bytes": map[string]any{
						"type":        "integer",
						"description": "Max response body bytes to download (default: 50000, max: 200000)",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Request timeout in seconds (default: 10, max: 60)",
					},
				},
				"required": []string{"url"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			URL            string `json:"url"`
			MaxBytes       int    `json:"max_bytes"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		rawURL := strings.TrimSpace(in.URL)
		if rawURL == "" {
			return "", fmt.Errorf("url is required")
		}
		maxBytes := in.MaxBytes
		if maxBytes <= 0 {
			maxBytes = 50000
		}
		if maxBytes > 200000 {
			maxBytes = 200000
		}
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 10
		}
		if timeout > 60 {
			timeout = 60
		}

		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		parsed, err := validateFetchURL(rawURL)
		if err != nil {
			return "", err
		}
		if err := validateResolvedHost(ctx, parsed.Hostname()); err != nil {
			return "", err
		}

		client := newSafeHTTPClient(time.Duration(timeout) * time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("User-Agent", "Ollama-Lab/1.0 (+local tool)")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.7")

		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		limited := io.LimitReader(resp.Body, int64(maxBytes)+1)
		bodyBytes, err := io.ReadAll(limited)
		if err != nil {
			return "", err
		}
		downloadTruncated := false
		if len(bodyBytes) > maxBytes {
			bodyBytes = bodyBytes[:maxBytes]
			downloadTruncated = true
		}

		contentType := resp.Header.Get("Content-Type")
		finalURL := resp.Request.URL.String()
		extracted := extractWebContent(bodyBytes, contentType, resp.Request.URL)

		result := map[string]any{
			"url":                rawURL,
			"final_url":          finalURL,
			"status":             resp.StatusCode,
			"status_text":        resp.Status,
			"content_type":       contentType,
			"title":              extracted.Title,
			"headings":           extracted.Headings,
			"text":               extracted.Text,
			"links":              extracted.Links,
			"truncated":          downloadTruncated || extracted.TextTruncated,
			"download_truncated": downloadTruncated,
			"text_truncated":     extracted.TextTruncated,
			"extraction_warning": extracted.Warning,
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}

func newSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: timeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if err := validateResolvedHost(ctx, host); err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			if _, err := validateFetchURL(req.URL.String()); err != nil {
				return err
			}
			if err := validateResolvedHost(req.Context(), req.URL.Hostname()); err != nil {
				return err
			}
			return nil
		},
	}
}

func validateFetchURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url must start with http:// or https://")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, fmt.Errorf("url host is required")
	}
	lowerHost := strings.ToLower(host)
	if !allowPrivateNetworksForTests && (lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost")) {
		return nil, fmt.Errorf("blocked host: %s", host)
	}
	if ip := net.ParseIP(host); ip != nil && isBlockedIP(ip) {
		return nil, fmt.Errorf("blocked IP: %s", host)
	}
	return u, nil
}

func validateResolvedHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("empty host")
	}
	lowerHost := strings.ToLower(host)
	if !allowPrivateNetworksForTests && (lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost")) {
		return fmt.Errorf("blocked host: %s", host)
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("blocked IP: %s", host)
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve host %s: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("resolve host %s: no addresses", host)
	}
	for _, ipa := range ips {
		if isBlockedIP(ipa.IP) {
			return fmt.Errorf("blocked resolved IP %s for host %s", ipa.IP.String(), host)
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if allowPrivateNetworksForTests {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// Metadata / special ranges
	if ip4 := ip.To4(); ip4 != nil {
		// 169.254.169.254 metadata, 100.64.0.0/10 CGNAT, 0.0.0.0/8, 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24, 198.18.0.0/15
		if ip4[0] == 0 {
			return true
		}
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return true
		}
		if ip4[0] == 198 && (ip4[1] == 51 || ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return true
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		return false
	}
	// IPv6: fc00::/7 unique local, fe80::/10 link-local already caught, ::1 loopback caught, :: unspecified caught
	if len(ip) == net.IPv6len {
		if ip[0]&0xfe == 0xfc {
			return true
		}
	}
	return false
}

type extractedContent struct {
	Title         string
	Headings      []string
	Text          string
	Links         []map[string]string
	TextTruncated bool
	Warning       string
}

var allowPrivateNetworksForTests = false

var (
	reScript     = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reStyle      = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reNoscript   = regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`)
	reSVG        = regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`)
	reTemplate   = regexp.MustCompile(`(?is)<template\b[^>]*>.*?</template>`)
	reIframe     = regexp.MustCompile(`(?is)<iframe\b[^>]*>.*?</iframe>`)
	reCanvas     = regexp.MustCompile(`(?is)<canvas\b[^>]*>.*?</canvas>`)
	reNav        = regexp.MustCompile(`(?is)<nav\b[^>]*>.*?</nav>`)
	reFooter     = regexp.MustCompile(`(?is)<footer\b[^>]*>.*?</footer>`)
	reHeader     = regexp.MustCompile(`(?is)<header\b[^>]*>.*?</header>`)
	reAside      = regexp.MustCompile(`(?is)<aside\b[^>]*>.*?</aside>`)
	reForm       = regexp.MustCompile(`(?is)<form\b[^>]*>.*?</form>`)
	reTitle      = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	reHeadings   = regexp.MustCompile(`(?is)<h([1-6])\b[^>]*>(.*?)</h[1-6]>`)
	reAnchor     = regexp.MustCompile(`(?is)<a\b[^>]*href\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))[^>]*>(.*?)</a>`)
	reTag        = regexp.MustCompile(`(?is)<[^>]+>`)
	reWhitespace = regexp.MustCompile(`[ \t\r\f\v]+`)
	reBlankLines = regexp.MustCompile(`\n{3,}`)
	reBlockBreak = regexp.MustCompile(`(?is)</(p|div|li|ul|ol|section|article|main|h[1-6]|tr|br|table|blockquote|pre)>|<(br|hr)\b[^>]*/?>`)
)

func extractWebContent(body []byte, contentType string, base *url.URL) extractedContent {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "application/json") || strings.Contains(ct, "text/plain") || strings.Contains(ct, "application/xml") || strings.Contains(ct, "text/xml") {
		text := strings.TrimSpace(string(body))
		text, trunc := truncateString(text, 20000)
		return extractedContent{Text: text, TextTruncated: trunc}
	}
	if contentType != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml+xml") && !strings.Contains(ct, "text/") {
		return extractedContent{Warning: fmt.Sprintf("unsupported content type for text extraction: %s", contentType)}
	}

	raw := string(body)
	title := cleanInlineText(firstMatchGroup(reTitle, raw, 1))
	headings := extractHeadings(raw)
	links := extractLinks(raw, base)

	cleaned := raw
	for _, re := range []*regexp.Regexp{reScript, reStyle, reNoscript, reSVG, reTemplate, reIframe, reCanvas, reNav, reFooter, reHeader, reAside, reForm} {
		cleaned = re.ReplaceAllString(cleaned, " ")
	}
	// Prefer main/article content if present.
	if main := extractMainBlock(cleaned); main != "" {
		cleaned = main
	}
	cleaned = reBlockBreak.ReplaceAllString(cleaned, "\n")
	cleaned = reTag.ReplaceAllString(cleaned, " ")
	cleaned = html.UnescapeString(cleaned)
	text := normalizeExtractedText(cleaned)
	text, trunc := truncateString(text, 20000)

	warning := ""
	if len(strings.TrimSpace(text)) < 80 && len(raw) > 2000 {
		warning = "Content extraction produced little readable text; page may be JavaScript-heavy or not primarily textual."
	}

	return extractedContent{
		Title:         title,
		Headings:      headings,
		Text:          text,
		Links:         links,
		TextTruncated: trunc,
		Warning:       warning,
	}
}

func extractMainBlock(s string) string {
	for _, tag := range []string{"main", "article"} {
		re := regexp.MustCompile(`(?is)<` + tag + `\b[^>]*>(.*?)</` + tag + `>`)
		m := re.FindStringSubmatch(s)
		if len(m) > 1 && len(strings.TrimSpace(reTag.ReplaceAllString(m[1], " "))) > 80 {
			return m[1]
		}
	}
	return ""
}

func extractHeadings(raw string) []string {
	matches := reHeadings.FindAllStringSubmatch(raw, 20)
	out := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		h := cleanInlineText(m[2])
		if h == "" {
			continue
		}
		key := strings.ToLower(h)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, h)
		if len(out) >= 15 {
			break
		}
	}
	return out
}

func extractLinks(raw string, base *url.URL) []map[string]string {
	matches := reAnchor.FindAllStringSubmatch(raw, 100)
	out := make([]map[string]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		href := m[2]
		if href == "" {
			href = m[3]
		}
		if href == "" {
			href = m[4]
		}
		href = strings.TrimSpace(html.UnescapeString(href))
		if href == "" || strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "mailto:") {
			continue
		}
		u, err := url.Parse(href)
		if err != nil {
			continue
		}
		if base != nil {
			u = base.ResolveReference(u)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			continue
		}
		linkText := cleanInlineText(m[5])
		key := u.String() + "|" + strings.ToLower(linkText)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		item := map[string]string{"url": u.String()}
		if linkText != "" {
			item["text"] = linkText
		}
		out = append(out, item)
		if len(out) >= 20 {
			break
		}
	}
	return out
}

func firstMatchGroup(re *regexp.Regexp, s string, group int) string {
	m := re.FindStringSubmatch(s)
	if len(m) > group {
		return m[group]
	}
	return ""
}

func cleanInlineText(s string) string {
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = reWhitespace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func normalizeExtractedText(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = reWhitespace.ReplaceAllString(line, " ")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	text := strings.Join(out, "\n")
	text = reBlankLines.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
