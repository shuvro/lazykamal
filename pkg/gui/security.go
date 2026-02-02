package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Security-related utilities for the GUI

// validatePath ensures a path is safe to access
// It resolves symlinks and ensures the path is within the expected base directory
func validatePath(basePath, targetPath string) error {
	// Resolve the base path
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return fmt.Errorf("invalid base path: %w", err)
	}

	// Resolve the target path
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	// Evaluate symlinks to get the real path
	realBase, err := filepath.EvalSymlinks(absBase)
	if err != nil {
		// Base might not exist yet, use absBase
		realBase = absBase
	}

	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		// Target might not exist yet, check parent
		parentDir := filepath.Dir(absTarget)
		realParent, parentErr := filepath.EvalSymlinks(parentDir)
		if parentErr != nil {
			realParent = parentDir
		}
		realTarget = filepath.Join(realParent, filepath.Base(absTarget))
	}

	// Ensure the target is within the base directory
	if !strings.HasPrefix(realTarget, realBase) {
		return fmt.Errorf("path traversal detected: %s is outside %s", targetPath, basePath)
	}

	return nil
}

// isSymlink checks if a path is a symbolic link
func isSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

// sanitizeLogLine removes potentially sensitive information from log output
// This is a basic implementation - extend based on your needs
func sanitizeLogLine(line string) string {
	// List of patterns that might indicate sensitive data
	sensitivePatterns := []string{
		"password=",
		"PASSWORD=",
		"secret=",
		"SECRET=",
		"token=",
		"TOKEN=",
		"api_key=",
		"API_KEY=",
		"KAMAL_REGISTRY_PASSWORD=",
		"RAILS_MASTER_KEY=",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(line, pattern) {
			// Find the pattern and mask the value
			idx := strings.Index(line, pattern)
			if idx != -1 {
				// Find end of value (space, newline, or end of string)
				endIdx := len(line)
				for i := idx + len(pattern); i < len(line); i++ {
					if line[i] == ' ' || line[i] == '\n' || line[i] == '\t' || line[i] == '"' || line[i] == '\'' {
						endIdx = i
						break
					}
				}
				line = line[:idx+len(pattern)] + "[REDACTED]" + line[endIdx:]
			}
		}
	}

	return line
}

// secureCreateDir creates a directory with secure permissions (0700)
func secureCreateDir(path string) error {
	return os.MkdirAll(path, 0700)
}

// secureWriteFile writes a file with secure permissions (0600)
func secureWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}

// validateCwd validates that a working directory path is safe
func validateCwd(path string) error {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("path does not exist: %s", path)
	}

	// Ensure it's a directory
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// Check for symlink attacks
	if isSymlink(path) {
		// Resolve the symlink and use the real path
		realPath, evalErr := filepath.EvalSymlinks(path)
		if evalErr != nil {
			return fmt.Errorf("cannot resolve symlink: %w", evalErr)
		}
		targetInfo, statErr := os.Stat(realPath)
		if statErr != nil {
			return fmt.Errorf("symlink target does not exist: %s", realPath)
		}
		if !targetInfo.IsDir() {
			return fmt.Errorf("symlink target is not a directory: %s", realPath)
		}
	}

	return nil
}
