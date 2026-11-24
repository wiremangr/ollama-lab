package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func registerFileWrite(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "file_write",
			Description: "Write content to a local file. Supports overwrite or append mode and utf8/base64 content encoding.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":        map[string]any{"type": "string", "description": "Path to the local file to write."},
					"content":     map[string]any{"type": "string", "description": "Content to write."},
					"encoding":    map[string]any{"type": "string", "description": "Content encoding: utf8 (default) or base64.", "enum": []string{"utf8", "base64"}},
					"mode":        map[string]any{"type": "string", "description": "Write mode: overwrite (default) or append.", "enum": []string{"overwrite", "append"}},
					"create_dirs": map[string]any{"type": "boolean", "description": "Create parent directories if needed (default: false)."},
				},
				"required": []string{"path", "content"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		_ = ctx
		var in struct {
			Path       string `json:"path"`
			Content    string `json:"content"`
			Encoding   string `json:"encoding"`
			Mode       string `json:"mode"`
			CreateDirs bool   `json:"create_dirs"`
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
		mode := strings.ToLower(strings.TrimSpace(in.Mode))
		if mode == "" {
			mode = "overwrite"
		}
		if encoding != "utf8" && encoding != "base64" {
			return "", fmt.Errorf("unsupported encoding: %s", encoding)
		}
		if mode != "overwrite" && mode != "append" {
			return "", fmt.Errorf("unsupported mode: %s", mode)
		}

		var data []byte
		if encoding == "base64" {
			b, err := base64.StdEncoding.DecodeString(in.Content)
			if err != nil {
				return "", fmt.Errorf("invalid base64 content: %w", err)
			}
			data = b
		} else {
			data = []byte(in.Content)
		}

		if in.CreateDirs {
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return "", err
			}
		}

		flags := os.O_CREATE | os.O_WRONLY
		if mode == "append" {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}
		f, err := os.OpenFile(path, flags, 0644)
		if err != nil {
			return "", err
		}
		defer f.Close()

		n, err := f.Write(data)
		if err != nil {
			return "", err
		}

		result := map[string]any{
			"ok":            true,
			"path":          path,
			"mode":          mode,
			"encoding":      encoding,
			"bytes_written": n,
		}
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}
