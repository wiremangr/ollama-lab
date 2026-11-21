package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ollama-lab/tools"
)

type OllamaClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewOllamaClient(baseURL string) *OllamaClient {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:11434"
	}
	return &OllamaClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 0, // No global timeout for long LLM generation
		},
	}
}

type ModelInfo struct {
	Name       string       `json:"name"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details"`
}

type ModelDetails struct {
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type TagsResponse struct {
	Models []ModelInfo `json:"models"`
}

func (c *OllamaClient) Ping(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/version", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code %d", resp.StatusCode)
	}

	var data struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "running", nil
	}
	return data.Version, nil
}

func (c *OllamaClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.BaseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama error (%d): %s", resp.StatusCode, string(body))
	}

	var tagsResp TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, err
	}
	return tagsResp.Models, nil
}

// OllamaOptions matches the Ollama parameters
type OllamaOptions struct {
	NumCtx           *int     `json:"num_ctx,omitempty"`
	NumPredict       *int     `json:"num_predict,omitempty"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	TopK             *int     `json:"top_k,omitempty"`
	RepeatPenalty    *float64 `json:"repeat_penalty,omitempty"`
	PresencePenalty  *float64 `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
	Seed             *int     `json:"seed,omitempty"`
	Stop             []string `json:"stop,omitempty"`
}

type ToolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ChatMessage struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolName  string     `json:"tool_name,omitempty"`
}

type ChatRequest struct {
	Model     string           `json:"model"`
	Messages  []ChatMessage    `json:"messages"`
	Stream    bool             `json:"stream"`
	Think     *bool            `json:"think,omitempty"`
	KeepAlive any              `json:"keep_alive,omitempty"`
	Options   *OllamaOptions   `json:"options,omitempty"`
	Tools     []tools.Definition `json:"tools,omitempty"`
}

type ChatResponseChunk struct {
	Model      string      `json:"model"`
	CreatedAt  time.Time   `json:"created_at"`
	Message    ChatMessage `json:"message"`
	Done       bool        `json:"done"`
	DoneReason string      `json:"done_reason,omitempty"`

	// Metrics when done is true (nanoseconds or counts)
	TotalDuration      int64 `json:"total_duration,omitempty"`
	LoadDuration       int64 `json:"load_duration,omitempty"`
	PromptEvalCount    int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          int   `json:"eval_count,omitempty"`
	EvalDuration       int64 `json:"eval_duration,omitempty"`
}

type ClientChunk struct {
	Type       string             `json:"type,omitempty"`
	Name       string             `json:"name,omitempty"`
	Arguments  any                `json:"arguments,omitempty"`
	Content    string             `json:"content,omitempty"`
	Delta      string             `json:"delta,omitempty"`
	Thinking   string             `json:"thinking,omitempty"`
	Done       bool               `json:"done"`
	DoneReason string             `json:"done_reason,omitempty"`
	Metrics    *GenerationMetrics `json:"metrics,omitempty"`
	Error      string             `json:"error,omitempty"`
}

type GenerationMetrics struct {
	TotalDurationMs  float64 `json:"total_duration_ms"`
	LoadDurationMs   float64 `json:"load_duration_ms"`
	PromptTokens     int     `json:"prompt_tokens"`
	PromptEvalSec    float64 `json:"prompt_eval_sec"`
	PromptTokensPerS float64 `json:"prompt_tokens_per_s"`
	OutputTokens     int     `json:"output_tokens"`
	EvalSec          float64 `json:"eval_sec"`
	OutputTokensPerS float64 `json:"output_tokens_per_s"`
	TimeToFirstTokMs float64 `json:"time_to_first_token_ms"`
}

func (c *OllamaClient) StreamChat(ctx context.Context, chatReq *ChatRequest, onChunk func(chunk ClientChunk) error) (*ChatMessage, *GenerationMetrics, error) {
	chatReq.Stream = true
	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return nil, nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/api/chat", bytes.NewReader(reqBody))
	if err != nil {
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	startTime := time.Now()
	var firstTokenTime time.Time
	gotFirstToken := false
	assistant := ChatMessage{Role: "assistant"}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("ollama API error (%d): %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			var chunk ChatResponseChunk
			if jsonErr := json.Unmarshal(line, &chunk); jsonErr == nil {
				if !gotFirstToken && (chunk.Message.Content != "" || chunk.Message.Thinking != "" || len(chunk.Message.ToolCalls) > 0) {
					gotFirstToken = true
					firstTokenTime = time.Now()
				}

				if chunk.Message.Content != "" {
					assistant.Content += chunk.Message.Content
				}
				if chunk.Message.Thinking != "" {
					assistant.Thinking += chunk.Message.Thinking
				}
				if len(chunk.Message.ToolCalls) > 0 {
					assistant.ToolCalls = mergeToolCalls(assistant.ToolCalls, chunk.Message.ToolCalls)
				}

				if chunk.Done {
					var ttft float64
					if gotFirstToken {
						ttft = float64(firstTokenTime.Sub(startTime).Milliseconds())
					}

					var promptSpeed float64
					if chunk.PromptEvalDuration > 0 {
						promptSpeed = float64(chunk.PromptEvalCount) / (float64(chunk.PromptEvalDuration) / 1e9)
					}

					var outputSpeed float64
					if chunk.EvalDuration > 0 {
						outputSpeed = float64(chunk.EvalCount) / (float64(chunk.EvalDuration) / 1e9)
					}

					metrics := &GenerationMetrics{
						TotalDurationMs:  float64(chunk.TotalDuration) / 1e6,
						LoadDurationMs:   float64(chunk.LoadDuration) / 1e6,
						PromptTokens:     chunk.PromptEvalCount,
						PromptEvalSec:    float64(chunk.PromptEvalDuration) / 1e9,
						PromptTokensPerS: promptSpeed,
						OutputTokens:     chunk.EvalCount,
						EvalSec:          float64(chunk.EvalDuration) / 1e9,
						OutputTokensPerS: outputSpeed,
						TimeToFirstTokMs: ttft,
					}

					if len(assistant.ToolCalls) == 0 {
						if err := onChunk(ClientChunk{
							Delta:      chunk.Message.Content,
							Thinking:   chunk.Message.Thinking,
							Done:       true,
							DoneReason: chunk.DoneReason,
							Metrics:    metrics,
						}); err != nil {
							return &assistant, metrics, err
						}
					} else if chunk.Message.Content != "" || chunk.Message.Thinking != "" {
						if err := onChunk(ClientChunk{
							Delta:    chunk.Message.Content,
							Thinking: chunk.Message.Thinking,
							Done:     false,
						}); err != nil {
							return &assistant, metrics, err
						}
					}
					return &assistant, metrics, nil
				}

				if err := onChunk(ClientChunk{
					Delta:    chunk.Message.Content,
					Thinking: chunk.Message.Thinking,
					Done:     false,
				}); err != nil {
					return &assistant, nil, err
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return &assistant, nil, err
		}
	}

	return &assistant, nil, nil
}

func mergeToolCalls(existing, incoming []ToolCall) []ToolCall {
	if len(existing) == 0 {
		out := make([]ToolCall, len(incoming))
		copy(out, incoming)
		return out
	}
	out := make([]ToolCall, len(existing))
	copy(out, existing)
	for i, call := range incoming {
		if i < len(out) {
			if call.ID != "" {
				out[i].ID = call.ID
			}
			if call.Type != "" {
				out[i].Type = call.Type
			}
			if call.Function.Name != "" {
				out[i].Function.Name = call.Function.Name
			}
			if len(call.Function.Arguments) > 0 {
				out[i].Function.Arguments = call.Function.Arguments
			}
		} else {
			out = append(out, call)
		}
	}
	return out
}
