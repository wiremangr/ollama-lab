//go:build !darwin

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func runDesktopWindow(title, url string, width, height int) {
	fmt.Printf("Ollama Lab is running at: %s\n", url)
	// Open in default browser on non-macOS
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "linux":
		cmd = "xdg-open"
		args = []string{url}
	default:
		return
	}

	_ = exec.Command(cmd, args...).Start()
}
