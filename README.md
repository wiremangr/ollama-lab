🧪 Ollama Lab (macOS Desktop App in Go)

A standalone, lightweight macOS desktop application written in Go for testing local Ollama models, fine-tuning inference parameters (num_predict, num_ctx, temperature, top_p, seed), and real-time system prompt benchmarking.

✨ Key Features

1. Native macOS Window:
  ⚬ Powered by built-in Apple frameworks (Cocoa & WebKit) via CGO, with zero dependency on Electron or Node.js.
  ⚬ Tiny ~6MB binary size with minimal resource consumption.
  ⚬ macOS dark-mode aesthetic with Apple SF Pro typography.
2. Complete Inference Parameter Control:
  ⚬ think (Model Thinking ON / OFF): Enable or disable reasoning / chain-of-thought processing for models like DeepSeek-R1. When ON, thoughts are rendered in a sleek collapsible accordion; when OFF, the model responds instantly without reasoning latency!
  ⚬ keep_alive (Model in RAM): Configure how long the model stays loaded in GPU/RAM (5m, 15m, 30m, 1h, or Always / -1 to keep it warm indefinitely).
  ⚬ num_ctx (Context Window): 2K, 4K, 8K, 16K, 32K or custom slider.
  ⚬ num_predict (Max Output Tokens): 256, 512, 1K, 2K, Max (-1).
  ⚬ temperature: 0.00 (Deterministic) to 2.00 (Creative).
  ⚬ top_p & top_k: Fine-sampling controls.
  ⚬ repeat_penalty: Prevent phrase repetition.
  ⚬ seed: Fixed seed support for 100% reproducible tests.
3. Dark & Light Mode:
  ⚬ Quick toggle button (🌙 / ☀️) in the header.
  ⚬ Automatic preference saving and synchronization with macOS system appearance.
4. System Prompt Presets:
  ⚬ Built-in presets: Senior Go Architect, Strict JSON Generator, Concise Expert, Code Reviewer, Greek Specialist.
  ⚬ Custom preset saving and loading (~/.ollama-lab/presets.json).
5. Real-time Telemetry & Metrics:
  ⚬ ⚡ Tokens / sec (generation speed)
  ⚬ ⏱️ Time to First Token (TTFT) (latency in milliseconds)
  ⚬ ⏳ Total Generation Duration
  ⚬ 🔢 Token Count (input vs output tokens)
  ⚬ 🧠 Prompt Evaluation Speed
6. A/B Compare Mode (Side-by-Side):
  ⚬ Test the exact same prompt simultaneously across 2 different models or with different system prompts/parameters.
  ⚬ Live parallel streaming to compare speed and response quality side-by-side.
  ⚬ Default RAM (Keep-Alive): 1h setting for both candidates to keep them warm in memory.
7. Single Test Context Retention (Interactive Chat):
  ⚬ With Keep context enabled, questions and answers form a continuous chat thread with timestamps and metrics, making it easy to track conversation evolution.
8. Channel Mode (Collaborative Multi-Agent Discussion):
  ⚬ Unified environment featuring 3 AI Agents discussing, analyzing, and interacting with each other and the user in a shared channel.
  ⚬ Model Selection: Global channel model with per-agent override capabilities.
  ⚬ Per-Agent Parameters: Configure `num_ctx` and `num_predict` individually for each agent.
  ⚬ Customized System Prompts: Distinct roles per agent (e.g., Debate with Visionary / Skeptic / Pragmatist, Code Architecture Team, or Dialectical Reasoning).
  ⚬ Shared History & Visibility: Every user message and agent response is injected into the context, instantly visible to all subsequent agents.
  ⚬ Sliding History Window: Configure how many recent channel messages are sent to agents (`All`, `Last 6`, `Last 10`, `Last 20`, `Last 40`).
  ⚬ Round Configuration: Set conversation cycles (1, 2, 3+ rounds) after which execution automatically pauses for user input.
  ⚬ Turn Order Control:
    ⚬ 1 ➔ 2 ➔ 3 (First to Third)
    ⚬ 3 ➔ 2 ➔ 1 (Third to First)
    ⚬ 🎲 Random (Randomized order ensuring the agent that spoke first in the current round never speaks first in the next round).
9. Tool Calling (All Views):
  ⚬ Configure enabled tools per view in Single, A/B Compare, and Channel via a Tools modal (Available / Enabled).
  ⚬ Backend tool registry in `tools/` with `GET /api/tools` and server-side tool execution loop during chat streaming.
  ⚬ SSE tool events (`tool_call`, `tool_result`, `tool_error`) are displayed in the UI.
  ⚬ Built-in tools: `get_current_time`, `ping`, `dns_lookup`, `web_page_fetch`, `connection_test`, `ssh_exec` (uses local `ssh`; authorized targets via `ssh_exec_auth.txt` or env var; explicit confirmation required), `tcp_client`, `tls_client` (session-based interactive TCP/TLS with `connect/send/recv/close`, `max_bytes`, optional `read_until`; TLS supports `insecure_skip_verify` with explicit confirmation), `file_read`, and `file_write`. Add your own tools in the `tools/` directory.

10. Zero External Bloat:
  ⚬ Built using only the Go Standard Library (net/http, embed, encoding/json) and native macOS Cocoa/WebKit frameworks.

🚀 How to Run

Option 1: Open as a macOS App

Double-click Ollama Lab.app in Finder, or launch via terminal:

cd path/to/ollama-lab
open "Ollama Lab.app"


Option 2: Run the Binary Directly

cd path/to/ollama-lab
./ollama-lab


Option 3: Headless / Browser Mode (Optional)

If you prefer running it solely as a local web server without a native window:

./ollama-lab -no-window -port 8080


Then open [http://127.0.0.1:8080](http://127.0.0.1:8080) in your favorite browser.

🛠️ Rebuild & Packaging

To rebuild the .app bundle:

./build-app.sh


Or a simple binary build:

go build -o ollama-lab .


🔧 Adding Tools

Tools are registered in the project-root `tools/` directory and automatically exposed to the UI via `GET /api/tools`.

1. Create a new file in `tools/`, e.g. `tools/my_tool.go`:

```go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

func registerMyTool(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name:        "my_tool",
			Description: "Describe what the tool does.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string", "description": "Example input"},
				},
				"required": []string{"input"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}

		out, _ := json.Marshal(map[string]any{"result": in.Input})
		return string(out), nil
	})
}
```

2. Register it in `tools/registry.go`:

```go
func (r *Registry) registerBuiltins() {
	registerGetCurrentTime(r)
	registerMyTool(r)
}
```

3. Rebuild the app:

```bash
./build-app.sh
```

💡 Keyboard Shortcuts

⚬ ⌘ + Enter: Instant prompt execution.
⚬ Escape: Immediate stream interruption / Stop.
<img width="1397" height="1063" alt="Single" src="https://github.com/user-attachments/assets/5a4021a0-d64b-4428-9416-202bdb960013" />
<img width="1397" height="1063" alt="A-B" src="https://github.com/user-attachments/assets/8245d342-4144-4c82-bbd8-6da6331214dc" />
<img width="1397" height="1063" alt="Channel" src="https://github.com/user-attachments/assets/5af69095-128c-4b59-9dc4-88b72af8156a" />


