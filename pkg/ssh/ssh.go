package ssh

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Client represents an SSH connection to a remote server
type Client struct {
	Host string
	User string
	Port string
}

// NewClient creates a new SSH client
func NewClient(host string) *Client {
	user := ""
	port := "22"

	// Parse user@host:port format
	if strings.Contains(host, "@") {
		parts := strings.SplitN(host, "@", 2)
		user = parts[0]
		host = parts[1]
	}

	if strings.Contains(host, ":") {
		parts := strings.SplitN(host, ":", 2)
		host = parts[0]
		port = parts[1]
	}

	return &Client{
		Host: host,
		User: user,
		Port: port,
	}
}

// Run executes a command on the remote server and returns the output
// Has a 30 second timeout to prevent hanging
func (c *Client) Run(command string) (string, error) {
	return c.RunWithTimeout(command, 30*time.Second)
}

// RunWithTimeout executes a command with a custom timeout
func (c *Client) RunWithTimeout(command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := c.buildSSHArgs()
	args = append(args, command)

	cmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out after %v", timeout)
	}
	if err != nil {
		// Include stderr in error message for debugging
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, stderr.String())
		}
		return "", err
	}

	return stdout.String(), nil
}

// RunStream executes a command and streams output line by line
func (c *Client) RunStream(command string, onLine func(string), stopCh <-chan struct{}) error {
	args := c.buildSSHArgs()
	args = append(args, command)

	cmd := exec.Command("ssh", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Read stdout and stderr in goroutines
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				lines := strings.Split(string(buf[:n]), "\n")
				for _, line := range lines {
					if line != "" {
						onLine(line)
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				lines := strings.Split(string(buf[:n]), "\n")
				for _, line := range lines {
					if line != "" {
						onLine(line)
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-stopCh:
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}

// TestConnection tests if SSH connection works
func (c *Client) TestConnection() error {
	_, err := c.Run("echo ok")
	return err
}

// buildSSHArgs builds the SSH command arguments
// Uses ControlMaster for connection multiplexing (reuses connections)
func (c *Client) buildSSHArgs() []string {
	// Control socket path for connection reuse
	controlPath := fmt.Sprintf("/tmp/lazykamal-ssh-%s", c.Host)

	args := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ConnectTimeout=10",
		// Connection multiplexing - reuse existing connections
		"-o", "ControlMaster=auto",
		"-o", fmt.Sprintf("ControlPath=%s", controlPath),
		"-o", "ControlPersist=60", // Keep connection alive for 60 seconds
	}

	if c.Port != "22" {
		args = append(args, "-p", c.Port)
	}

	target := c.Host
	if c.User != "" {
		target = c.User + "@" + c.Host
	}
	args = append(args, target)

	return args
}

// HostDisplay returns a display string for the host
func (c *Client) HostDisplay() string {
	if c.User != "" {
		return fmt.Sprintf("%s@%s", c.User, c.Host)
	}
	return c.Host
}

// DetectUser tries to detect the SSH user from config or defaults
func DetectUser(host string) string {
	// Try to get from SSH config
	cmd := exec.Command("ssh", "-G", host)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil {
		for _, line := range strings.Split(stdout.String(), "\n") {
			if strings.HasPrefix(line, "user ") {
				return strings.TrimPrefix(line, "user ")
			}
		}
	}

	// Default to current user
	if user := os.Getenv("USER"); user != "" {
		return user
	}

	return "root"
}
