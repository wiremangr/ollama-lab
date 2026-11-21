package tools

import (
	"context"
	"encoding/json"
)

// Definition describes a tool exposed to the model (Ollama/OpenAI-style function tool).
type Definition struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// Function describes the callable function name, description, and JSON schema parameters.
type Function struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// Handler executes a tool call. args contains the raw JSON arguments from the model.
// Return a human/model-readable string result (JSON is recommended).
type Handler func(ctx context.Context, args json.RawMessage) (string, error)
