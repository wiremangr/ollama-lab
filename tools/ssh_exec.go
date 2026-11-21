package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func registerSSHExec(r *Registry) {
	_ = r.Register(Definition{
		Type: "function",
		Function: Function{
			Name: "ssh_exec",
			Description: "Execute a command on a remote system via the local ssh client. " +
				"Use this only when the user explicitly asks to run a command on a remote host. " +
				"Targets must be explicitly authorized in ssh_exec_auth.txt (project root, one IP/hostname per line, reloaded on each call) or OLLAMA_LAB_SSH_ALLOWED_TARGETS. " +
				"Before execution, call with confirm=false to get a confirmation request, then only call with confirm=true after the user explicitly confirms.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"host": map[string]any{
						"type":        "string",
						"description": "Remote hostname or IP address (private IPs are allowed if authorized).",
					},
					"user": map[string]any{
						"type":        "string",
						"description": "SSH username. Optional if configured in SSH config, but recommended.",
					},
					"port": map[string]any{
						"type":        "integer",
						"description": "SSH port (default: 22).",
					},
					"command": map[string]any{
						"type":        "string",
						"description": "The remote command to execute.",
					},
					"identity_file": map[string]any{
						"type":        "string",
						"description": "Optional path to SSH private key file.",
					},
					"timeout_seconds": map[string]any{
						"type":        "integer",
						"description": "Overall execution timeout in seconds (default: 30, max: 120).",
					},
					"strict_host_key_checking": map[string]any{
						"type":        "string",
						"description": "SSH StrictHostKeyChecking value: accept-new, yes, or no (default: accept-new).",
						"enum":        []string{"accept-new", "yes", "no"},
					},
					"confirm": map[string]any{
						"type":        "boolean",
						"description": "Must be true only after the user explicitly confirms executing this remote command. If false or omitted, the tool returns a confirmation_required response and does not execute.",
					},
				},
				"required": []string{"host", "command", "confirm"},
			},
		},
	}, func(ctx context.Context, args json.RawMessage) (string, error) {
		var in struct {
			Host                  string `json:"host"`
			User                  string `json:"user"`
			Port                  int    `json:"port"`
			Command               string `json:"command"`
			IdentityFile          string `json:"identity_file"`
			TimeoutSeconds        int    `json:"timeout_seconds"`
			StrictHostKeyChecking string `json:"strict_host_key_checking"`
			Confirm               bool   `json:"confirm"`
		}
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid args: %w", err)
		}

		host := strings.TrimSpace(in.Host)
		user := strings.TrimSpace(in.User)
		command := strings.TrimSpace(in.Command)
		identityFile := strings.TrimSpace(in.IdentityFile)
		if host == "" {
			return "", fmt.Errorf("host is required")
		}
		if command == "" {
			return "", fmt.Errorf("command is required")
		}
		if strings.ContainsAny(host, " \t\r\n;&|`$><()") || strings.ContainsAny(user, " \t\r\n;&|`$><()@") {
			return "", fmt.Errorf("invalid host or user")
		}
		if identityFile != "" && strings.ContainsAny(identityFile, "\r\n`$") {
			return "", fmt.Errorf("invalid identity_file")
		}

		port := in.Port
		if port == 0 {
			port = 22
		}
		if port <= 0 || port > 65535 {
			return "", fmt.Errorf("port must be between 1 and 65535")
		}
		timeout := in.TimeoutSeconds
		if timeout <= 0 {
			timeout = 30
		}
		if timeout > 120 {
			timeout = 120
		}
		strict := strings.TrimSpace(in.StrictHostKeyChecking)
		if strict == "" {
			strict = "accept-new"
		}
		switch strict {
		case "accept-new", "yes", "no":
		default:
			return "", fmt.Errorf("strict_host_key_checking must be accept-new, yes, or no")
		}

		target := host
		if user != "" {
			target = user + "@" + host
		}

		allowed, allowErr := isSSHAuthorizedTarget(host, user, port)
		if !allowed {
			result := map[string]any{
				"ok":     false,
				"status": "unauthorized_target",
				"host":   host,
				"user":   user,
				"port":   port,
				"target": target,
				"error":  allowErr,
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		}

		if !in.Confirm {
			result := map[string]any{
				"ok":      false,
				"status":  "confirmation_required",
				"message": "Explicit user confirmation is required before executing remote SSH commands. Ask the user to confirm, then call ssh_exec again with confirm=true.",
				"host":    host,
				"user":    user,
				"port":    port,
				"target":  target,
				"command": command,
				"timeout_seconds": timeout,
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		}

		cmdArgs := []string{
			"-o", "BatchMode=yes",
			"-o", fmt.Sprintf("ConnectTimeout=%d", min(timeout, 30)),
			"-o", "StrictHostKeyChecking=" + strict,
			"-p", fmt.Sprintf("%d", port),
		}
		if identityFile != "" {
			cmdArgs = append(cmdArgs, "-i", identityFile)
		}
		cmdArgs = append(cmdArgs, target, "--", command)

		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()

		stdoutStr, stdoutTrunc := truncateString(stdout.String(), 20000)
		stderrStr, stderrTrunc := truncateString(stderr.String(), 20000)

		result := map[string]any{
			"ok":               err == nil,
			"status":           "executed",
			"host":             host,
			"user":             user,
			"port":             port,
			"target":           target,
			"command":          command,
			"stdout":           strings.TrimSpace(stdoutStr),
			"stderr":           strings.TrimSpace(stderrStr),
			"stdout_truncated": stdoutTrunc,
			"stderr_truncated": stderrTrunc,
		}
		if err != nil {
			result["error"] = err.Error()
			if exitErr, ok := err.(*exec.ExitError); ok {
				result["exit_code"] = exitErr.ExitCode()
			} else {
				result["exit_code"] = nil
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		}
		result["exit_code"] = 0
		b, _ := json.Marshal(result)
		return string(b), nil
	})
}

func isSSHAuthorizedTarget(host, user string, port int) (bool, string) {
	fileEntries, fileErr, fileFound, checkedPaths := readSSHAuthFile()
	envRaw := strings.TrimSpace(os.Getenv("OLLAMA_LAB_SSH_ALLOWED_TARGETS"))

	hasAny := len(fileEntries) > 0 || envRaw != ""
	if !hasAny {
		checked := ""
		if len(checkedPaths) > 0 {
			checked = "; checked: " + strings.Join(checkedPaths, ", ")
		}
		if fileFound && fileErr != nil {
			return false, fmt.Sprintf("SSH target not authorized: failed to read ssh_exec_auth.txt: %v%s", fileErr, checked)
		}
		if fileFound {
			return false, fmt.Sprintf("SSH target not authorized: ssh_exec_auth.txt was found but contains no approved entries%s", checked)
		}
		return false, fmt.Sprintf("SSH target not authorized: add approved IPs/hostnames to ssh_exec_auth.txt (one per line) or set OLLAMA_LAB_SSH_ALLOWED_TARGETS%s", checked)
	}

	for _, entry := range fileEntries {
		if matchSSHAllowedTarget(entry, host, user, port) {
			return true, ""
		}
	}

	if envRaw != "" {
		for _, entry := range strings.Split(envRaw, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if matchSSHAllowedTarget(entry, host, user, port) {
				return true, ""
			}
		}
	}

	if fileFound && fileErr != nil {
		return false, fmt.Sprintf("SSH target %s is not authorized (also failed to read ssh_exec_auth.txt: %v)", formatSSHTarget(host, user, port), fileErr)
	}
	return false, fmt.Sprintf("SSH target %s is not authorized. Add it to ssh_exec_auth.txt or OLLAMA_LAB_SSH_ALLOWED_TARGETS", formatSSHTarget(host, user, port))
}

func readSSHAuthFile() ([]string, error, bool, []string) {
	paths := []string{}
	cwd, _ := os.Getwd()
	if cwd != "" {
		paths = append(paths, filepathJoin(cwd, "ssh_exec_auth.txt"))
	}
	paths = append(paths, "ssh_exec_auth.txt")
	if exe, err := os.Executable(); err == nil {
		exeDir := filepathDir(exe)
		paths = append(paths, filepathJoin(exeDir, "ssh_exec_auth.txt"))
		// For .app bundle: Contents/MacOS/ollama-lab -> go up to folder containing .app
		paths = append(paths, filepathJoin(filepathDir(filepathDir(filepathDir(exeDir))), "ssh_exec_auth.txt"))
	}
	// Deduplicate while preserving order
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}

	var lastErr error
	foundAny := false
	for _, p := range uniq {
		b, err := os.ReadFile(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			lastErr = err
			foundAny = true
			continue
		}
		foundAny = true
		lines := strings.Split(string(b), "\n")
		entries := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			entries = append(entries, line)
		}
		return entries, nil, true, uniq
	}
	if lastErr != nil {
		return nil, lastErr, foundAny, uniq
	}
	return nil, nil, foundAny, uniq
}

func filepathJoin(a, b string) string {
	if a == "" {
		return b
	}
	if strings.HasSuffix(a, "/") {
		return a + b
	}
	return a + "/" + b
}

func filepathDir(p string) string {
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "."
	}
	if i == 0 {
		return "/"
	}
	return p[:i]
}

func matchSSHAllowedTarget(entry, host, user string, port int) bool {
	eUser, eHost, ePort, ok := parseSSHTargetEntry(entry)
	if !ok {
		return false
	}
	if eHost != "*" && !strings.EqualFold(eHost, host) {
		return false
	}
	if eUser != "" && eUser != "*" && !strings.EqualFold(eUser, user) {
		return false
	}
	if ePort != 0 && ePort != port {
		return false
	}
	// If entry specifies user, require user to be present.
	if eUser != "" && eUser != "*" && user == "" {
		return false
	}
	return true
}

func parseSSHTargetEntry(entry string) (user, host string, port int, ok bool) {
	s := strings.TrimSpace(entry)
	if s == "" {
		return "", "", 0, false
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		user = strings.TrimSpace(s[:at])
		s = strings.TrimSpace(s[at+1:])
	}
	// host:port (simple form; IPv6 with port should be [ipv6]:port)
	if strings.HasPrefix(s, "[") {
		rb := strings.Index(s, "]")
		if rb == -1 {
			return "", "", 0, false
		}
		host = strings.TrimSpace(s[1:rb])
		rest := strings.TrimSpace(s[rb+1:])
		if strings.HasPrefix(rest, ":") {
			p, err := fmt.Sscanf(rest[1:], "%d", &port)
			if err != nil || p != 1 {
				return "", "", 0, false
			}
		}
	} else if c := strings.LastIndex(s, ":"); c > -1 && strings.Count(s, ":") == 1 {
		host = strings.TrimSpace(s[:c])
		if _, err := fmt.Sscanf(s[c+1:], "%d", &port); err != nil {
			return "", "", 0, false
		}
	} else {
		host = s
	}
	if host == "" {
		return "", "", 0, false
	}
	if port < 0 || port > 65535 {
		return "", "", 0, false
	}
	return user, host, port, true
}

func formatSSHTarget(host, user string, port int) string {
	t := host
	if user != "" {
		t = user + "@" + host
	}
	if port != 0 {
		t = fmt.Sprintf("%s:%d", t, port)
	}
	return t
}

func truncateString(s string, max int) (string, bool) {
	if len(s) <= max {
		return s, false
	}
	return s[:max], true
}
