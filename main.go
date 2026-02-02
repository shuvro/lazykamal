package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/jroimartin/gocui"
	"github.com/shuvro/lazykamal/pkg/gui"
)

var version = "dev"

// checkKamalInstalled verifies that kamal CLI is available on PATH
func checkKamalInstalled() error {
	_, err := exec.LookPath("kamal")
	if err != nil {
		return fmt.Errorf("kamal CLI not found on PATH.\n\nPlease install Kamal first:\n  gem install kamal\n\nOr see: https://kamal-deploy.org/docs/installation/")
	}
	return nil
}

func main() {
	// Handle --version flag
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("lazykamal", version)
		os.Exit(0)
	}

	// Handle --help flag
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printHelp()
		os.Exit(0)
	}

	// Check that kamal is installed before starting the TUI
	if err := checkKamalInstalled(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	g, err := gui.New(version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}

	// Set working directory if provided
	if len(os.Args) > 1 && os.Args[1] != "" && os.Args[1][0] != '-' {
		if err := g.SetCwd(os.Args[1]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	}

	// Setup graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Run in goroutine so we can handle signals
	errCh := make(chan error, 1)
	go func() {
		errCh <- g.Run()
	}()

	// Wait for either GUI exit or signal
	select {
	case err := <-errCh:
		if err != nil && err != gocui.ErrQuit {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "\nReceived %s, shutting down...\n", sig)
		os.Exit(0)
	}
}

func printHelp() {
	fmt.Println(`Lazykamal - A lazydocker-style TUI for Kamal deployments

Usage:
  lazykamal [path]      Start TUI in the specified directory
  lazykamal             Start TUI in the current directory

Options:
  -h, --help            Show this help message
  -v, --version         Show version information

Keyboard Shortcuts:
  ↑/↓         Navigate menus
  Enter       Select item / Execute command
  m           Open main menu
  b / Esc     Go back
  r           Refresh destinations
  c           Clear log
  ?           Show help overlay
  q           Quit

For more information, visit: https://github.com/shuvro/lazykamal`)
}
