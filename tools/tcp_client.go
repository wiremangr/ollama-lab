package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func registerTCPClient(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name: "tcp_client",
			Description: "Interactive TCP client with session support. Actions: connect, send, recv, close. " +
				"Use connect to open a session, then send/recv with session_id, then close when done.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action": map[string]any{
						"type":        "string",
						"description": "Action to perform: connect, send, recv, close.",
						"enum":        []string{"connect", "send", "recv", "close"},
					},
					"host":            map[string]any{"type": "string", "description": "Remote host/IP for connect."},
					"port":            map[string]any{"type": "integer", "description": "Remote TCP port for connect."},
					"session_id":      map[string]any{"type": "string", "description": "Session ID returned by connect (for send/recv/close)."},
					"data":            map[string]any{"type": "string", "description": "Data to send (for send)."},
					"encoding":        map[string]any{"type": "string", "description": "Data encoding: utf8 (default) or base64.", "enum": []string{"utf8", "base64"}},
					"max_bytes":       map[string]any{"type": "integer", "description": "Max bytes to receive (default: 4096, max: 65536)."},
					"read_until":      map[string]any{"type": "string", "description": "Optional delimiter; recv reads until delimiter, max_bytes, timeout, or EOF."},
					"timeout_seconds": map[string]any{"type": "integer", "description": "Operation timeout in seconds (default: 10, max: 60)."},
				},
				"required": []string{"action"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Action         string `json:"action"`
			Host           string `json:"host"`
			Port           int    `json:"port"`
			SessionID      string `json:"session_id"`
			Data           string `json:"data"`
			Encoding       string `json:"encoding"`
			MaxBytes       int    `json:"max_bytes"`
			ReadUntil      string `json:"read_until"`
			TimeoutSeconds int    `json:"timeout_seconds"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		action := strings.ToLower(strings.TrimSpace(in.Action))
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 10
		}
		if timeout > 60 {
			timeout = 60
		}

		switch action {
		case "connect":
			host := strings.TrimSpace(in.Host)
			if host == "" {
				return "", fmt.Errorf("host is required for connect")
			}
			if in.Port <= 0 || in.Port > 65535 {
				return "", fmt.Errorf("port must be between 1 and 65535")
			}
			conn, err := dialTCP(ctx, host, in.Port, time.Duration(timeout)*time.Second)
			if err != nil {
				return netToolError("tcp_client", action, "", err)
			}
			sess, err := globalNetSessions.open("tcp", conn)
			if err != nil {
				_ = conn.Close()
				return netToolError("tcp_client", action, "", err)
			}
			return netToolResult(map[string]any{
				"ok": true, "tool": "tcp_client", "action": action, "status": "connected",
				"session_id": sess.id, "remote_address": sess.remoteAddr, "local_address": sess.localAddr,
			})
		case "send":
			sess, err := globalNetSessions.get(strings.TrimSpace(in.SessionID))
			if err != nil {
				return netToolError("tcp_client", action, in.SessionID, err)
			}
			payload, err := decodeNetData(in.Data, in.Encoding)
			if err != nil {
				return netToolError("tcp_client", action, sess.id, err)
			}
			n, err := sess.send(ctx, payload, time.Duration(timeout)*time.Second)
			if err != nil {
				return netToolError("tcp_client", action, sess.id, err)
			}
			return netToolResult(map[string]any{
				"ok": true, "tool": "tcp_client", "action": action, "status": "sent",
				"session_id": sess.id, "bytes_sent": n,
			})
		case "recv":
			sess, err := globalNetSessions.get(strings.TrimSpace(in.SessionID))
			if err != nil {
				return netToolError("tcp_client", action, in.SessionID, err)
			}
			maxBytes := in.MaxBytes
			if maxBytes <= 0 {
				maxBytes = 4096
			}
			if maxBytes > netMaxReadBytes {
				maxBytes = netMaxReadBytes
			}
			data, eof, timedOut, err := sess.recv(ctx, maxBytes, time.Duration(timeout)*time.Second, in.ReadUntil)
			if err != nil {
				return netToolError("tcp_client", action, sess.id, err)
			}
			encoded, err := encodeNetData(data, in.Encoding)
			if err != nil {
				return netToolError("tcp_client", action, sess.id, err)
			}
			return netToolResult(map[string]any{
				"ok": true, "tool": "tcp_client", "action": action, "status": "received",
				"session_id": sess.id, "data": encoded, "encoding": normalizeNetEncoding(in.Encoding),
				"bytes_received": len(data), "eof": eof, "timed_out": timedOut,
			})
		case "close":
			sessionID := strings.TrimSpace(in.SessionID)
			if sessionID == "" {
				return "", fmt.Errorf("session_id is required for close")
			}
			if err := globalNetSessions.close(sessionID); err != nil {
				return netToolError("tcp_client", action, sessionID, err)
			}
			return netToolResult(map[string]any{
				"ok": true, "tool": "tcp_client", "action": action, "status": "closed", "session_id": sessionID,
			})
		default:
			return "", fmt.Errorf("unsupported action: %s", action)
		}
	})
}

func netToolResult(m map[string]any) (string, error) {
	b, _ := json.Marshal(m)
	return string(b), nil
}

func netToolError(tool, action, sessionID string, err error) (string, error) {
	m := map[string]any{"ok": false, "tool": tool, "action": action, "error": err.Error()}
	if sessionID != "" {
		m["session_id"] = sessionID
	}
	b, _ := json.Marshal(m)
	return string(b), nil
}

func normalizeNetEncoding(enc string) string {
	e := strings.ToLower(strings.TrimSpace(enc))
	if e == "base64" {
		return "base64"
	}
	return "utf8"
}
