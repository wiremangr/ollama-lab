package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func registerFileRead(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "file_read",
			Description: "Read a local file from the filesystem. Returns file content as utf8 or base64, with size and truncation metadata.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string", "description": "Path to the local file to read."},
					"encoding":  map[string]any{"type": "string", "description": "Output encoding: utf8 (default) or base64.", "enum": []string{"utf8", "base64"}},
					"offset":    map[string]any{"type": "integer", "description": "Byte offset to start reading from (default: 0)."},
					"max_bytes": map[string]any{"type": "integer", "description": "Maximum bytes to read (default: 20000, max: 100000)."},
				},
				"required": []string{"path"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		_ = ctx
		var in struct {
			Path     string `json:"path"`
			Encoding string `json:"encoding"`
			Offset   int64  `json:"offset"`
			MaxBytes int    `json:"max_bytes"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}
		path := strings.TrimSpace(in.Path)
		if path == "" {
			return "", fmt.Errorf("path is required")
		}
		encoding := strings.ToLower(strings.TrimSpace(in.Encoding))
		if encoding == "" {
			encoding = "utf8"
		}
		if encoding != "utf8" && encoding != "base64" {
			return "", fmt.Errorf("unsupported encoding: %s", encoding)
		}
		if in.Offset < 0 {
			return "", fmt.Errorf("offset must be >= 0")
		}
		maxBytes := in.MaxBytes
		if maxBytes <= 0 {
			maxBytes = 20000
		}
		if maxBytes > 100000 {
			maxBytes = 100000
		}

		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			return "", fmt.Errorf("path is a directory: %s", path)
		}

		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer f.Close()

		if in.Offset > 0 {
			if _, err := f.Seek(in.Offset, 0); err != nil {
				return "", err
			}
		}

		buf := make([]byte, maxBytes+1)
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		buf = buf[:n]
		truncated := false
		if len(buf) > maxBytes {
			buf = buf[:maxBytes]
			truncated = true
		}

		var data string
		if encoding == "base64" {
			data = base64.StdEncoding.EncodeToString(buf)
		} else {
			data = string(buf)
		}

		result := map[string]any{
			"ok":         true,
			"path":       path,
			"size":       info.Size(),
			"offset":     in.Offset,
			"encoding":   encoding,
			"data":       data,
			"bytes_read": len(buf),
			"truncated":  truncated,
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}
