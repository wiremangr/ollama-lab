package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

type Server struct {
	Ollama        *OllamaClient
	Presets       *PresetStore
	StaticFS      fs.FS
	ServerBaseURL string
}

func NewServer(ollamaURL string, staticFS fs.FS) *Server {
	return &Server{
		Ollama:   NewOllamaClient(ollamaURL),
		Presets:  NewPresetStore(),
		StaticFS: staticFS,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/models", s.handleModels)
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
		if (message.Role == "user" || message.Role == "assistant") && strings.TrimSpace(message.Content) != "" {
			messages = append(messages, ChatMessage{Role: message.Role, Content: message.Content})
		}
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

	chatReq := &ChatRequest{
		Model:     payload.Model,
		Messages:  messages,
		Think:     payload.Think,
		KeepAlive: payload.KeepAlive,
		Options:   payload.Options,
	}

	// Set headers for Server-Sent Events
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	err := s.Ollama.StreamChat(r.Context(), chatReq, func(chunk ClientChunk) error {
		data, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		_, writeErr := fmt.Fprintf(w, "data: %s\n\n", data)
		if writeErr != nil {
			return writeErr
		}
		flusher.Flush()
		return nil
	})

	if err != nil && r.Context().Err() == nil {
		errChunk, _ := json.Marshal(ClientChunk{
			Done:  true,
			Error: err.Error(),
		})
		fmt.Fprintf(w, "data: %s\n\n", errChunk)
		flusher.Flush()
	}
}
