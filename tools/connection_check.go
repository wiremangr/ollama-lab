package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

func registerConnectionTest(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "connection_test",
			Description: "Test TCP connectivity to a remote host and port, reporting reachability and latency.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host":            map[string]any{"type": "string", "description": "Hostname or IP address"},
					"port":            map[string]any{"type": "integer", "description": "TCP port number"},
					"timeout_seconds": map[string]any{"type": "integer", "description": "Connection timeout in seconds (default: 5, max: 30)"},
				},
				"required": []string{"host", "port"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Host           string `json:"host"`
			Port           int    `json:"port"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		host := strings.TrimSpace(in.Host)
		if host == "" {
			return "", fmt.Errorf("host is required")
		}
		if in.Port <= 0 || in.Port > 65535 {
			return "", fmt.Errorf("port must be between 1 and 65535")
		}
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 5
		}
		if timeout > 30 {
			timeout = 30
		}

		address := net.JoinHostPort(host, fmt.Sprintf("%d", in.Port))
		dialer := net.Dialer{Timeout: time.Duration(timeout) * time.Second}
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", address)
		latency := time.Since(start)

		result := map[string]any{
			"host":       host,
			"port":       in.Port,
			"address":    address,
			"latency_ms": latency.Milliseconds(),
		}
		if err != nil {
			result["reachable"] = false
			result["error"] = err.Error()
			b, _ := json.Marshal(result)
			return string(b), nil
		}
		defer conn.Close()
		result["reachable"] = true
		result["local_address"] = conn.LocalAddr().String()
		result["remote_address"] = conn.RemoteAddr().String()
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}
