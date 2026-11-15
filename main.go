package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"ollama-lab/server"
)

//go:embed web/*
var webFiles embed.FS

func main() {
	portFlag := flag.Int("port", 0, "Port to run the internal server on (0 for dynamic free port)")
	ollamaURL := flag.String("ollama", "http://127.0.0.1:11434", "Ollama API server URL")
	noWindow := flag.Bool("no-window", false, "Run in headless mode (do not open native macOS window)")
	flag.Parse()

	// Sub-FS for web content
	webContent, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("Failed to load embedded web assets: %v", err)
	}

	// Create Ollama Lab Server
	labServer := server.NewServer(*ollamaURL, webContent)

	// Bind TCP listener
	addr := fmt.Sprintf("127.0.0.1:%d", *portFlag)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to bind TCP listener on %s: %v", addr, err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	appURL := fmt.Sprintf("http://127.0.0.1:%d", actualPort)

	// Run HTTP server in background goroutine
	httpServer := &http.Server{
		Handler: labServer.Routes(),
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	fmt.Println("==================================================")
	fmt.Println("  🧪 Ollama Lab - macOS Model & Prompt Playground ")
	fmt.Println("==================================================")
	fmt.Printf("  • Local App URL : %s\n", appURL)
	fmt.Printf("  • Ollama Backend: %s\n", *ollamaURL)
	fmt.Println("==================================================")

	if *noWindow {
		fmt.Println("Running in headless mode. Press Ctrl+C to quit.")
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		fmt.Println("\nShutting down Ollama Lab...")
		_ = httpServer.Close()
		return
	}

	// Launch native macOS desktop application window
	// This will block until the window is closed by the user
	runDesktopWindow("🧪 Ollama Lab", appURL, 1280, 850)
	_ = httpServer.Close()
}
