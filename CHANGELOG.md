# Changelog

## 2026-11-21

### Added

- **Channel: per-agent inference options**
  - Added per-agent `num_ctx` and `num_predict` controls to Channel agent cards, including dynamically added agents.
  - Channel requests now send each agent's configured options to `/api/chat`.

- **Channel: configurable history window**
  - Added a `History` selector in the Channel toolbar (`All`, `Last 6`, `Last 10`, `Last 20`, `Last 40`).
  - Channel history payloads now use the selected sliding window; `All` preserves the previous full-history behavior.
  - Window size counts channel history entries (user and agent messages).

- **Tool calling support (all views)**
  - Added backend tool registry/scaffolding in `tools/` (project root) for registering Go tool handlers.
  - Added `GET /api/tools` endpoint for listing available tool definitions.
  - Extended Ollama chat structs/stream handling to support tool definitions, tool calls, tool result messages, and SSE tool events.
  - Implemented server-side tool execution loop in `/api/chat` with SSE events (`tool_call`, `tool_result`, `tool_error`) and a max-iteration guard.
  - Added built-in tools: `get_current_time`, `ping`, `dns_lookup`, `web_page_fetch`, `connection_test`, and `ssh_exec` (local `ssh` command, no external Go dependency).
  - Improved `dns_lookup`: `type` is required; supports `A`, `AAAA`, `CNAME`, `MX`, `NS`, `TXT`, `PTR` (reverse DNS via `net.LookupAddr`, IP input required), and `ALL`; returns structured `records`/`errors`.
  - Improved `web_page_fetch`: extracts title/headings/readable text/links, removes scripts/styles/nav/footer/irrelevant markup, returns structured JSON, applies size limits and truncation metadata, reports JS-heavy/low-extraction cases, and preserves timeout handling with SSRF protections (private/loopback/metadata blocking and redirect validation).
  - Hardened `ssh_exec`: clearer model-facing schema/description, explicit `confirm` gate before remote execution, authorized targets via `ssh_exec_auth.txt` (one IP/hostname per line, reloaded each call; checked in cwd, executable dir, and .app bundle parent dir) and/or `OLLAMA_LAB_SSH_ALLOWED_TARGETS`, private IPs allowed when authorized, and separate `stdout`, `stderr`, and `exit_code` results. Created `ssh_exec_auth.txt` template in project root.
  - Added tests: `tools/dns_lookup_test.go`, `tools/web_page_fetch_test.go`.

- **Tools configuration UI (Single, A/B Compare, Channel)**
  - Added a Tools modal with dual-pane management: Available Tools / Enabled Tools.
  - Added Tools buttons/badges in Single, Compare, and Channel views.
  - Enabled tool selections are tracked per view and sent as `enabled_tools` in chat requests.
  - Frontend SSE rendering now displays tool call/result/error events in output streams.

### Changed

- **Tool code organization**
  - Moved tool registry/types/implementations from `server/tools.go` to project-root `tools/` directory.
  - New structure: `tools/types.go`, `tools/registry.go`, `tools/get_current_time.go`.
  - Adding new tools now requires creating a tool file in `tools/` and registering it in `tools/registry.go`.

- Rebuilt the macOS app bundle (`Ollama Lab.app`) to include the latest changes.
