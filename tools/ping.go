package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func registerPing(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "ping",
			Description: "Ping a host using the local ping command and return its output.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host":            map[string]any{"type": "string", "description": "Hostname or IP address to ping"},
					"count":           map[string]any{"type": "integer", "description": "Number of packets (default: 4, max: 10)"},
					"timeout_seconds": map[string]any{"type": "integer", "description": "Overall timeout in seconds (default: 10, max: 60)"},
				},
				"required": []string{"host"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Host           string `json:"host"`
			Count          int    `json:"count"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		host := strings.TrimSpace(in.Host)
		if host == "" {
			return "", fmt.Errorf("host is required")
		}
		if strings.ContainsAny(host, " \t\r\n;&|`$><()") {
			return "", fmt.Errorf("invalid host")
		}
		count := in.Count
		if count <= 0 {
			count = 4
		}
		if count > 10 {
			count = 10
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

		cmdArgs := []string{"-c", fmt.Sprintf("%d", count)}
		// macOS/BSD: -W is milliseconds; Linux: -W is seconds.
		perPacket := 2000
		if runtime.GOOS == "darwin" {
			cmdArgs = append(cmdArgs, "-W", fmt.Sprintf("%d", perPacket))
		} else {
			cmdArgs = append(cmdArgs, "-W", fmt.Sprintf("%d", perPacket/1000))
		}
		cmdArgs = append(cmdArgs, host)

		cmd := exec.CommandContext(ctx, "ping", cmdArgs...)
		out, err := cmd.CombinedOutput()
		result := map[string]any{
			"host":    host,
			"count":   count,
			"command": "ping " + strings.Join(cmdArgs, " "),
			"output":  strings.TrimSpace(string(out)),
		}
		if err != nil {
			result["ok"] = false
			result["error"] = err.Error()
			b, _ := json.Marshal(result)
			return string(b), nil
		}
		result["ok"] = true
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}
