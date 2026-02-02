package gui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a subdirectory
	subDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create a file
	testFile := filepath.Join(subDir, "deploy.yml")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name      string
		basePath  string
		target    string
		wantError bool
	}{
		{
			name:      "valid path within base",
			basePath:  tmpDir,
			target:    testFile,
			wantError: false,
		},
		{
			name:      "valid subdirectory",
			basePath:  tmpDir,
			target:    subDir,
			wantError: false,
		},
		{
			name:      "path traversal attempt",
			basePath:  subDir,
			target:    filepath.Join(subDir, "..", "..", "etc", "passwd"),
			wantError: true,
		},
		{
			name:      "absolute path outside base",
			basePath:  tmpDir,
			target:    "/etc/passwd",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.basePath, tt.target)
			if tt.wantError && err == nil {
				t.Errorf("validatePath() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("validatePath() unexpected error: %v", err)
			}
		})
	}
}

func TestSanitizeLogLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no sensitive data",
			input:    "Deploying application...",
			expected: "Deploying application...",
		},
		{
			name:     "password in line",
			input:    "Using password=mysecretpass for login",
			expected: "Using password=[REDACTED] for login",
		},
		{
			name:     "API key in line",
			input:    "API_KEY=abc123xyz",
			expected: "API_KEY=[REDACTED]",
		},
		{
			name:     "registry password",
			input:    "Using KAMAL_REGISTRY_PASSWORD=docker_token_here for login",
			expected: "Using KAMAL_REGISTRY_PASSWORD=[REDACTED] for login",
		},
		{
			name:     "multiple sensitive values",
			input:    "password=secret1 token=secret2",
			expected: "password=[REDACTED] token=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeLogLine(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeLogLine() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file
	regularFile := filepath.Join(tmpDir, "regular.txt")
	if err := os.WriteFile(regularFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	// Create a symlink
	symlinkPath := filepath.Join(tmpDir, "symlink.txt")
	if err := os.Symlink(regularFile, symlinkPath); err != nil {
		t.Skip("Symlinks not supported on this platform")
	}

	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"regular file", regularFile, false},
		{"symlink", symlinkPath, true},
		{"non-existent", filepath.Join(tmpDir, "nonexistent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSymlink(tt.path)
			if result != tt.expected {
				t.Errorf("isSymlink(%s) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestValidateCwd(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file (not a directory)
	regularFile := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(regularFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create file: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		wantError bool
	}{
		{"valid directory", tmpDir, false},
		{"non-existent path", filepath.Join(tmpDir, "nonexistent"), true},
		{"file instead of directory", regularFile, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCwd(tt.path)
			if tt.wantError && err == nil {
				t.Errorf("validateCwd() expected error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Errorf("validateCwd() unexpected error: %v", err)
			}
		})
	}
}

func TestSecureCreateDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "secure", "nested")

	err := secureCreateDir(testDir)
	if err != nil {
		t.Fatalf("secureCreateDir() error: %v", err)
	}

	// Check directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}

	if !info.IsDir() {
		t.Error("Created path is not a directory")
	}

	// Check permissions (on Unix)
	if info.Mode().Perm() != 0700 {
		t.Errorf("Directory permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestSecureWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "secrets")

	err := secureWriteFile(testFile, []byte("secret data"))
	if err != nil {
		t.Fatalf("secureWriteFile() error: %v", err)
	}

	// Check file exists
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("File not created: %v", err)
	}

	// Check permissions (on Unix)
	if info.Mode().Perm() != 0600 {
		t.Errorf("File permissions = %o, want 0600", info.Mode().Perm())
	}

	// Check content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(data) != "secret data" {
		t.Errorf("File content = %q, want %q", string(data), "secret data")
	}
}
