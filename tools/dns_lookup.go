package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

func registerDNSLookup(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "dns_lookup",
			Description: "Perform DNS lookups. Requires host and type. Supported types: A, AAAA, CNAME, MX, NS, TXT, PTR (reverse DNS; host must be an IP), and ALL. Returns structured JSON records.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type":        "string",
						"description": "Hostname for forward lookups, or IP address for PTR/reverse lookups.",
					},
					"type": map[string]any{
						"type":        "string",
						"description": "DNS record type to query. Required. One of: A, AAAA, CNAME, MX, NS, TXT, PTR, ALL.",
						"enum":        []string{"A", "AAAA", "CNAME", "MX", "NS", "TXT", "PTR", "ALL"},
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Lookup timeout in seconds (default: 5, max: 30).",
					},
				},
				"required": []string{"host", "type"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Host           string `json:"host"`
			Type           string `json:"type"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		host := strings.TrimSpace(in.Host)
		if host == "" {
			return "", fmt.Errorf("host is required")
		}
		recType := strings.ToUpper(strings.TrimSpace(in.Type))
		if recType == "" {
			return "", fmt.Errorf("type is required (A, AAAA, CNAME, MX, NS, TXT, PTR, ALL)")
		}
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 5
		}
		if timeout > 30 {
			timeout = 30
		}

		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
		resolver := net.DefaultResolver

		result := map[string]any{
			"host": host,
			"type": recType,
		}
		records := map[string]any{}
		var errs []string
		setRecord := func(key string, val any) {
			records[key] = val
		}
		setErr := func(key string, err error) {
			errs = append(errs, fmt.Sprintf("%s: %v", key, err))
		}

		lookupA := func() {
			ips, err := resolver.LookupIP(ctx, "ip4", host)
			if err != nil {
				setErr("A", err)
				return
			}
			out := make([]string, 0, len(ips))
			for _, ip := range ips {
				out = append(out, ip.String())
			}
			setRecord("A", out)
		}
		lookupAAAA := func() {
			ips, err := resolver.LookupIP(ctx, "ip6", host)
			if err != nil {
				setErr("AAAA", err)
				return
			}
			out := make([]string, 0, len(ips))
			for _, ip := range ips {
				out = append(out, ip.String())
			}
			setRecord("AAAA", out)
		}
		lookupCNAME := func() {
			cname, err := resolver.LookupCNAME(ctx, host)
			if err != nil {
				setErr("CNAME", err)
				return
			}
			setRecord("CNAME", cname)
		}
		lookupMX := func() {
			mxs, err := resolver.LookupMX(ctx, host)
			if err != nil {
				setErr("MX", err)
				return
			}
			out := make([]map[string]any, 0, len(mxs))
			for _, mx := range mxs {
				out = append(out, map[string]any{"host": mx.Host, "preference": mx.Pref})
			}
			setRecord("MX", out)
		}
		lookupNS := func() {
			nss, err := resolver.LookupNS(ctx, host)
			if err != nil {
				setErr("NS", err)
				return
			}
			out := make([]string, 0, len(nss))
			for _, ns := range nss {
				out = append(out, ns.Host)
			}
			setRecord("NS", out)
		}
		lookupTXT := func() {
			txts, err := resolver.LookupTXT(ctx, host)
			if err != nil {
				setErr("TXT", err)
				return
			}
			setRecord("TXT", txts)
		}
		lookupPTR := func() {
			ip := net.ParseIP(host)
			if ip == nil {
				setErr("PTR", fmt.Errorf("PTR lookup requires a valid IP address"))
				return
			}
			names, err := resolver.LookupAddr(ctx, host)
			if err != nil {
				setErr("PTR", err)
				return
			}
			setRecord("PTR", names)
		}

		switch recType {
		case "A":
			lookupA()
		case "AAAA":
			lookupAAAA()
		case "CNAME":
			lookupCNAME()
		case "MX":
			lookupMX()
		case "NS":
			lookupNS()
		case "TXT":
			lookupTXT()
		case "PTR":
			lookupPTR()
		case "ALL":
			if net.ParseIP(host) != nil {
				lookupPTR()
			}
			lookupA()
			lookupAAAA()
			lookupCNAME()
			lookupMX()
			lookupNS()
			lookupTXT()
		default:
			return "", fmt.Errorf("unsupported record type: %s (supported: A, AAAA, CNAME, MX, NS, TXT, PTR, ALL)", recType)
		}

		result["records"] = records
		if len(errs) > 0 {
			result["errors"] = errs
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}
