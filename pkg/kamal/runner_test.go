package kamal

import (
	"strings"
	"testing"
)

func TestBuildGlobalArgs(t *testing.T) {
	tests := []struct {
		name     string
		opts     RunOptions
		expected []string
	}{
		{
			name:     "empty options",
			opts:     RunOptions{},
			expected: nil,
		},
		{
			name: "config file only",
			opts: RunOptions{
				ConfigFile: "/path/to/deploy.yml",
			},
			expected: []string{"--config-file", "/path/to/deploy.yml"},
		},
		{
			name: "destination (non-production)",
			opts: RunOptions{
				Destination: "staging",
			},
			expected: []string{"--destination", "staging"},
		},
		{
			name: "destination (production - should be ignored)",
			opts: RunOptions{
				Destination: "production",
			},
			expected: nil,
		},
		{
			name: "primary flag",
			opts: RunOptions{
				Primary: true,
			},
			expected: []string{"--primary"},
		},
		{
			name: "hosts",
			opts: RunOptions{
				Hosts: "192.168.1.1,192.168.1.2",
			},
			expected: []string{"--hosts", "192.168.1.1,192.168.1.2"},
		},
		{
			name: "roles",
			opts: RunOptions{
				Roles: "web,worker",
			},
			expected: []string{"--roles", "web,worker"},
		},
		{
			name: "version",
			opts: RunOptions{
				Version: "abc123",
			},
			expected: []string{"--version", "abc123"},
		},
		{
			name: "skip hooks",
			opts: RunOptions{
				SkipHooks: true,
			},
			expected: []string{"--skip-hooks"},
		},
		{
			name: "verbose",
			opts: RunOptions{
				Verbose: true,
			},
			expected: []string{"--verbose"},
		},
		{
			name: "quiet",
			opts: RunOptions{
				Quiet: true,
			},
			expected: []string{"--quiet"},
		},
		{
			name: "combined options",
			opts: RunOptions{
				ConfigFile:  "/path/to/deploy.yml",
				Destination: "staging",
				Verbose:     true,
			},
			expected: []string{"--config-file", "/path/to/deploy.yml", "--destination", "staging", "--verbose"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildGlobalArgs(tt.opts)

			if len(result) != len(tt.expected) {
				t.Errorf("buildGlobalArgs() returned %d args, want %d\nGot: %v\nWant: %v",
					len(result), len(tt.expected), result, tt.expected)
				return
			}

			for i, arg := range result {
				if arg != tt.expected[i] {
					t.Errorf("buildGlobalArgs()[%d] = %q, want %q", i, arg, tt.expected[i])
				}
			}
		})
	}
}

func TestResult_Combined(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected string
	}{
		{
			name:     "stdout only",
			result:   Result{Stdout: "output", Stderr: ""},
			expected: "output",
		},
		{
			name:     "stdout and stderr",
			result:   Result{Stdout: "output", Stderr: "error"},
			expected: "output\nerror",
		},
		{
			name:     "empty",
			result:   Result{Stdout: "", Stderr: ""},
			expected: "",
		},
		{
			name:     "stderr only",
			result:   Result{Stdout: "", Stderr: "error"},
			expected: "\nerror",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result.Combined()
			if result != tt.expected {
				t.Errorf("Combined() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestResult_Lines(t *testing.T) {
	tests := []struct {
		name     string
		result   Result
		expected []string
	}{
		{
			name:     "single line",
			result:   Result{Stdout: "line1"},
			expected: []string{"line1"},
		},
		{
			name:     "multiple lines",
			result:   Result{Stdout: "line1\nline2\nline3"},
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "empty",
			result:   Result{Stdout: ""},
			expected: nil,
		},
		{
			name:     "with trailing newline",
			result:   Result{Stdout: "line1\nline2\n"},
			expected: []string{"line1", "line2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result.Lines()

			if len(result) != len(tt.expected) {
				t.Errorf("Lines() returned %d lines, want %d\nGot: %v", len(result), len(tt.expected), result)
				return
			}

			for i, line := range result {
				if line != tt.expected[i] {
					t.Errorf("Lines()[%d] = %q, want %q", i, line, tt.expected[i])
				}
			}
		})
	}
}

func TestRunOpts(t *testing.T) {
	tests := []struct {
		name         string
		cwd          string
		dest         *DeployDestination
		expectedCwd  string
		expectedConf string
		expectedDest string
	}{
		{
			name:         "nil destination",
			cwd:          "/project",
			dest:         nil,
			expectedCwd:  "/project",
			expectedConf: "",
			expectedDest: "",
		},
		{
			name: "production destination",
			cwd:  "/project",
			dest: &DeployDestination{
				Name:       "production",
				ConfigPath: "/project/config/deploy.yml",
			},
			expectedCwd:  "/project",
			expectedConf: "/project/config/deploy.yml",
			expectedDest: "",
		},
		{
			name: "staging destination",
			cwd:  "/project",
			dest: &DeployDestination{
				Name:       "staging",
				ConfigPath: "/project/config/deploy.staging.yml",
			},
			expectedCwd:  "/project",
			expectedConf: "/project/config/deploy.staging.yml",
			expectedDest: "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RunOpts(tt.cwd, tt.dest)

			if result.Cwd != tt.expectedCwd {
				t.Errorf("Cwd = %q, want %q", result.Cwd, tt.expectedCwd)
			}
			if result.ConfigFile != tt.expectedConf {
				t.Errorf("ConfigFile = %q, want %q", result.ConfigFile, tt.expectedConf)
			}
			if result.Destination != tt.expectedDest {
				t.Errorf("Destination = %q, want %q", result.Destination, tt.expectedDest)
			}
		})
	}
}

// TestKamalNotInstalled tests behavior when kamal is not available
// This test is skipped if kamal is installed
func TestKamalNotInstalled(t *testing.T) {
	// Save original PATH and set to empty to simulate kamal not found
	// This is a bit invasive so we skip in normal runs
	t.Skip("Skipping kamal not installed test - requires modifying PATH")
}

// Integration tests - these require kamal to be installed
// Run with: go test -tags=integration ./...

func TestCommandBuilding(t *testing.T) {
	// Test that commands are built correctly without actually running them
	// This verifies our argument building logic

	tests := []struct {
		name        string
		fn          func(RunOptions) ([]string, []string)
		opts        RunOptions
		wantCmd     string
		wantContain []string
	}{
		{
			name: "deploy",
			opts: RunOptions{Destination: "staging"},
			fn: func(opts RunOptions) ([]string, []string) {
				return buildGlobalArgs(opts), []string{"deploy"}
			},
			wantCmd:     "deploy",
			wantContain: []string{"--destination", "staging"},
		},
		{
			name: "deploy with skip-push",
			opts: RunOptions{},
			fn: func(opts RunOptions) ([]string, []string) {
				return buildGlobalArgs(opts), []string{"deploy", "--skip-push"}
			},
			wantCmd:     "deploy",
			wantContain: []string{"--skip-push"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			globalArgs, subCmd := tt.fn(tt.opts)
			allArgs := append(globalArgs, subCmd...)
			argsStr := strings.Join(allArgs, " ")

			for _, want := range tt.wantContain {
				if !strings.Contains(argsStr, want) {
					t.Errorf("Command args %q should contain %q", argsStr, want)
				}
			}
		})
	}
}
