package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func registerGetCurrentTime(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "get_current_time",
			Description: "Returns the current server time in RFC3339 format. Useful as a simple built-in test tool.",
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		_ = ctx
		_ = args
		return fmt.Sprintf(`{"time":"%s"}`, time.Now().Format(time.RFC3339)), nil
	})
}
