package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"ollama-lab/tools"
)

type Server struct {
	Ollama        *OllamaClient
	Presets       *PresetStore
	Tools         *tools.Registry
	StaticFS      fs.FS
	ServerBaseURL string
}

func NewServer(ollamaURL string, staticFS fs.FS) *Server {
	return &Server{
		Ollama:   NewOllamaClient(ollamaURL),
		Presets:  NewPresetStore(),
		Tools:    tools.NewRegistry(),
		StaticFS: staticFS,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/tools", s.handleTools)
	mux.HandleFunc("GET /api/presets", s.handleGetPresets)
	mux.HandleFunc("POST /api/presets", s.handleSavePreset)
	mux.HandleFunc("DELETE /api/presets", s.handleDeletePreset)
	mux.HandleFunc("POST /api/chat", s.handleChatStream)

	// Static UI assets
	if s.StaticFS != nil {
		fileServer := http.FileServer(http.FS(s.StaticFS))
		mux.Handle("/", fileServer)
	}

	return corsMiddleware(mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	version, err := s.Ollama.Ping(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		json.NewEncoder(w).Encode(map[string]any{
			"connected": false,
			"error":     err.Error(),
			"url":       s.Ollama.BaseURL,
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"connected": true,
		"version":   version,
		"url":       s.Ollama.BaseURL,
	})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.Ollama.ListModels(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(models)
}

func (s *Server) handleTools(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	defs := []tools.Definition{}
	if s.Tools != nil {
		defs = s.Tools.List()
	}
	json.NewEncoder(w).Encode(map[string]any{"tools": defs})
}

func (s *Server) handleGetPresets(w http.ResponseWriter, r *http.Request) {
	presets := s.Presets.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(presets)
}

func (s *Server) handleSavePreset(w http.ResponseWriter, r *http.Request) {
	var p Preset
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(p.Name) == "" {
		http.Error(w, "preset name is required", http.StatusBadRequest)
		return
	}

	if p.ID == "" {
		p.ID = strings.ToLower(strings.ReplaceAll(p.Name, " ", "-"))
	}

	if err := s.Presets.SavePreset(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing preset id", http.StatusBadRequest)
		return
	}

	if err := s.Presets.DeletePreset(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type ChatStreamPayload struct {
	Model        string         `json:"model"`
	SystemPrompt string         `json:"system_prompt"`
	UserPrompt   string         `json:"user_prompt"`
	History      []ChatMessage  `json:"history,omitempty"`
	Think        *bool          `json:"think,omitempty"`
	KeepAlive    any            `json:"keep_alive,omitempty"`
	Options      *OllamaOptions `json:"options,omitempty"`
	EnabledTools []string       `json:"enabled_tools,omitempty"`
}

func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var payload ChatStreamPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(payload.Model) == "" {
		http.Error(w, "model is required", http.StatusBadRequest)
		return
	}

	// Prepare messages
	messages := make([]ChatMessage, 0, len(payload.History)+2)
	if strings.TrimSpace(payload.SystemPrompt) != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: payload.SystemPrompt,
		})
	}
	for _, message := range payload.History {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		if role == "tool" {
			if strings.TrimSpace(message.Content) == "" && strings.TrimSpace(message.ToolName) == "" {
				continue
			}
			messages = append(messages, ChatMessage{Role: "tool", Content: message.Content, ToolName: message.ToolName})
			continue
		}
		if strings.TrimSpace(message.Content) == "" && len(message.ToolCalls) == 0 {
			continue
		}
		messages = append(messages, ChatMessage{
			Role:      role,
			Content:   message.Content,
			Thinking:  message.Thinking,
			ToolCalls: message.ToolCalls,
			ToolName:  message.ToolName,
		})
	}
	if strings.TrimSpace(payload.UserPrompt) != "" {
		messages = append(messages, ChatMessage{
			Role:    "user",
			Content: payload.UserPrompt,
		})
	}

	if len(messages) == 0 {
		http.Error(w, "at least one message or prompt is required", http.StatusBadRequest)
		return
	}

	var toolDefs []tools.Definition
	if len(payload.EnabledTools) > 0 && s.Tools != nil {
		defs, err := s.Tools.Definitions(payload.EnabledTools)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		toolDefs = defs
	}

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sendChunk := func(chunk ClientChunk) error {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	const maxToolIterations = 8
	for iteration := 0; iteration < maxToolIterations; iteration++ {
		if r.Context().Err() != nil {
			return
		}

		chatReq := &ChatRequest{
			Model:     payload.Model,
			Messages:  messages,
			Think:     payload.Think,
			KeepAlive: payload.KeepAlive,
			Options:   payload.Options,
			Tools:     toolDefs,
		}

		assistant, metrics, err := s.Ollama.StreamChat(r.Context(), chatReq, func(chunk ClientChunk) error {
			return sendChunk(chunk)
		})
		if err != nil {
			if r.Context().Err() == nil {
				_ = sendChunk(ClientChunk{Done: true, Error: err.Error()})
			}
			return
		}

		if assistant == nil || len(assistant.ToolCalls) == 0 {
			if metrics != nil && assistant != nil {
				// Final done already emitted by StreamChat when no tool calls.
				_ = assistant
			}
			return
		}

		// Emit tool calls and execute them
		messages = append(messages, *assistant)
		for _, tc := range assistant.ToolCalls {
			_ = sendChunk(ClientChunk{
				Type:      "tool_call",
				Name:      tc.Function.Name,
				Arguments: string(tc.Function.Arguments),
			})

			result, execErr := s.Tools.Execute(r.Context(), tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				errMsg := execErr.Error()
				_ = sendChunk(ClientChunk{
					Type:  "tool_error",
					Name:  tc.Function.Name,
					Error: errMsg,
				})
				messages = append(messages, ChatMessage{
					Role:     "tool",
					ToolName: tc.Function.Name,
					Content:  fmt.Sprintf("Error: %s", errMsg),
				})
				continue
			}

			_ = sendChunk(ClientChunk{
				Type:    "tool_result",
				Name:    tc.Function.Name,
				Content: result,
			})
			messages = append(messages, ChatMessage{
				Role:     "tool",
				ToolName: tc.Function.Name,
				Content:  result,
			})
		}

		// Continue loop: model gets tool results and can respond/further call tools.
		if iteration == maxToolIterations-1 {
			_ = sendChunk(ClientChunk{Done: true, Error: "tool calling iteration limit reached"})
			return
		}
	}

	_ = sendChunk(ClientChunk{Done: true, Error: "tool calling loop ended unexpectedly"})
}
