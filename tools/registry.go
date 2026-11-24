package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type registeredTool struct {
	definition Definition
	handler    Handler
}

// Registry holds all available tools and their handlers.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

// NewRegistry creates a registry and registers built-in tools.
func NewRegistry() *Registry {
	r := &Registry{tools: make(map[string]registeredTool)}
	r.registerBuiltins()
	return r
}

// Register adds or replaces a tool definition + handler.
func (r *Registry) Register(def Definition, handler Handler) error {
	if def.Type == "" {
		def.Type = "function"
	}
	if def.Function.Name == "" {
		return fmt.Errorf("tool function name is required")
	}
	if handler == nil {
		return fmt.Errorf("tool handler is required: %s", def.Function.Name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[def.Function.Name] = registeredTool{definition: def, handler: handler}
	return nil
}

// List returns all registered tool definitions, sorted by name.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].definition)
	}
	return defs
}

// Definitions resolves enabled tool names into definitions.
func (r *Registry) Definitions(names []string) ([]Definition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(names) == 0 {
		return nil, nil
	}
	defs := make([]Definition, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		tool, ok := r.tools[name]
		if !ok {
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
		defs = append(defs, tool.definition)
	}
	return defs, nil
}

// Execute runs a registered tool handler by name.
func (r *Registry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	if len(args) == 0 {
		args = json.RawMessage([]byte("{}"))
	}
	return tool.handler(ctx, args)
}

// registerBuiltins registers all built-in tools.
// To add a new tool, create a file in this directory (e.g. tools/my_tool.go)
// and register it here.
func (r *Registry) registerBuiltins() {
	registerGetCurrentTime(r)
	registerPing(r)
	registerDNSLookup(r)
	registerWebPageFetch(r)
	registerConnectionTest(r)
	registerSSHExec(r)
	registerTCPClient(r)
	registerTLSClient(r)
	registerFileRead(r)
	registerFileWrite(r)
}
