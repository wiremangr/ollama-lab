package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Preset struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	SystemPrompt string         `json:"system_prompt"`
	Options      *OllamaOptions `json:"options,omitempty"`
	IsBuiltin    bool           `json:"is_builtin"`
}

type PresetStore struct {
	mu       sync.RWMutex
	filePath string
	presets  []Preset
}

func defaultInt(val int) *int {
	return &val
}

func defaultFloat(val float64) *float64 {
	return &val
}

func getBuiltinPresets() []Preset {
	return []Preset{
		{
			ID:          "general-assistant",
			Name:        "General Helpful Assistant",
			Description: "Balanced, polite and accurate general assistant",
			SystemPrompt: "You are a helpful, respectful, and honest assistant. Always answer as helpfully as possible while being direct and safe.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(4096),
				NumPredict:  defaultInt(1024),
				Temperature: defaultFloat(0.7),
				TopP:        defaultFloat(0.9),
			},
			IsBuiltin: true,
		},
		{
			ID:          "go-architect",
			Name:        "Senior Go Architect",
			Description: "Writes clean, idiomatic, concurrent Go code with comments",
			SystemPrompt: "You are a Principal Software Engineer specializing in Go (Golang). Provide idiomatic, clean, memory-efficient, and concurrent Go code. Follow standard library conventions, handle all errors gracefully, and explain key architectural choices.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(8192),
				NumPredict:  defaultInt(2048),
				Temperature: defaultFloat(0.2), // Low temp for code precision
				TopP:        defaultFloat(0.95),
			},
			IsBuiltin: true,
		},
		{
			ID:          "concise-expert",
			Name:        "Concise Expert (Zero Fluff)",
			Description: "Direct, no preamble, bullet-pointed precision answers",
			SystemPrompt: "Answer directly without conversational filler, intro greetings, or concluding remarks. Deliver factual, concise, and structured answers immediately.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(4096),
				NumPredict:  defaultInt(512),
				Temperature: defaultFloat(0.3),
				TopP:        defaultFloat(0.9),
			},
			IsBuiltin: true,
		},
		{
			ID:          "strict-json",
			Name:        "Strict JSON Generator",
			Description: "Outputs ONLY valid raw JSON with no markdown wrapping",
			SystemPrompt: "You are a strict data transformation engine. Output ONLY valid, parsable JSON. Do not include markdown codeblocks (no ```json), do not include any explanatory text before or after.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(4096),
				NumPredict:  defaultInt(1024),
				Temperature: defaultFloat(0.1),
				TopP:        defaultFloat(0.9),
			},
			IsBuiltin: true,
		},
		{
			ID:          "code-reviewer",
			Name:        "Code Reviewer & Bug Hunter",
			Description: "Analyzes code for vulnerabilities, edge cases, and performance bottlenecks",
			SystemPrompt: "You are an expert security auditor and senior code reviewer. Analyze provided code for: 1) Logic and edge-case bugs, 2) Concurrency/race hazards, 3) Memory leaks/performance bottlenecks, 4) Security issues. Format feedback into 'Issue', 'Impact', and 'Suggested Fix'.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(8192),
				NumPredict:  defaultInt(2048),
				Temperature: defaultFloat(0.3),
				TopP:        defaultFloat(0.9),
			},
			IsBuiltin: true,
		},
		{
			ID:          "greek-specialist",
			Name:        "Greek Language Specialist",
			Description: "Fluent, natural, grammatically flawless Greek",
			SystemPrompt: "Είσαι ένας εξειδικευμένος βοηθός με άριστη γνώση της ελληνικής γλώσσας. Απαντάς πάντοτε σε άπταιστα, φυσικά και γραμματικώς ορθά ελληνικά, με ακρίβεια και σαφήνεια.",
			Options: &OllamaOptions{
				NumCtx:      defaultInt(4096),
				NumPredict:  defaultInt(1024),
				Temperature: defaultFloat(0.7),
				TopP:        defaultFloat(0.9),
			},
			IsBuiltin: true,
		},
	}
}

func NewPresetStore() *PresetStore {
	home, err := os.UserHomeDir()
	var filePath string
	if err == nil {
		dir := filepath.Join(home, ".ollama-lab")
		_ = os.MkdirAll(dir, 0755)
		filePath = filepath.Join(dir, "presets.json")
	}

	store := &PresetStore{
		filePath: filePath,
		presets:  getBuiltinPresets(),
	}

	store.loadCustom()
	return store
}

func (s *PresetStore) loadCustom() {
	if s.filePath == "" {
		return
	}
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return
	}

	var customPresets []Preset
	if err := json.Unmarshal(data, &customPresets); err == nil {
		for _, cp := range customPresets {
			cp.IsBuiltin = false
			s.presets = append(s.presets, cp)
		}
	}
}

func (s *PresetStore) saveCustom() error {
	if s.filePath == "" {
		return nil
	}

	var custom []Preset
	for _, p := range s.presets {
		if !p.IsBuiltin {
			custom = append(custom, p)
		}
	}

	data, err := json.MarshalIndent(custom, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filePath, data, 0644)
}

func (s *PresetStore) GetAll() []Preset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Preset, len(s.presets))
	copy(res, s.presets)
	return res
}

func (s *PresetStore) SavePreset(p Preset) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	p.IsBuiltin = false
	found := false
	for i, existing := range s.presets {
		if existing.ID == p.ID {
			if existing.IsBuiltin {
				// Don't overwrite built-in flag
				p.ID = p.ID + "-custom"
			} else {
				s.presets[i] = p
				found = true
				break
			}
		}
	}

	if !found {
		s.presets = append(s.presets, p)
	}

	return s.saveCustom()
}

func (s *PresetStore) DeletePreset(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := make([]Preset, 0, len(s.presets))
	for _, p := range s.presets {
		if p.ID == id && !p.IsBuiltin {
			continue
		}
		filtered = append(filtered, p)
	}
	s.presets = filtered
	return s.saveCustom()
}
