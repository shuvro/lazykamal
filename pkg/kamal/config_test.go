package kamal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDeployConfigs(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	tests := []struct {
		name          string
		files         map[string]string
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "no config files",
			files:         map[string]string{},
			expectedCount: 0,
			expectedNames: nil,
		},
		{
			name: "single base config only",
			files: map[string]string{
				"deploy.yml": "service: myapp\n",
			},
			expectedCount: 1,
			expectedNames: []string{""},
		},
		{
			name: "base config with staging destination",
			files: map[string]string{
				"deploy.yml":         "service: myapp\n",
				"deploy.staging.yml": "service: myapp-staging\n",
			},
			expectedCount: 1,
			expectedNames: []string{"staging"},
		},
		{
			name: "yaml extension",
			files: map[string]string{
				"deploy.yaml": "service: myapp\n",
			},
			expectedCount: 1,
			expectedNames: []string{""},
		},
		{
			name: "multiple destinations",
			files: map[string]string{
				"deploy.yml":            "service: myapp\n",
				"deploy.staging.yml":    "service: myapp-staging\n",
				"deploy.production.yml": "service: myapp-prod\n",
			},
			expectedCount: 2,
			expectedNames: []string{"staging", "production"},
		},
		{
			name: "ignores non-deploy files",
			files: map[string]string{
				"deploy.yml":    "service: myapp\n",
				"database.yml":  "adapter: postgresql\n",
				"settings.yml":  "key: value\n",
				"deploy.backup": "old: config\n",
			},
			expectedCount: 1,
			expectedNames: []string{""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up config directory
			entries, _ := os.ReadDir(configDir)
			for _, e := range entries {
				os.Remove(filepath.Join(configDir, e.Name()))
			}

			// Create test files
			for name, content := range tt.files {
				path := filepath.Join(configDir, name)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("Failed to create file %s: %v", name, err)
				}
			}

			// Run test
			configs, err := FindDeployConfigs(tmpDir)
			if err != nil {
				t.Fatalf("FindDeployConfigs() error = %v", err)
			}

			if len(configs) != tt.expectedCount {
				t.Errorf("FindDeployConfigs() returned %d configs, want %d", len(configs), tt.expectedCount)
			}

			// Check that expected names are present
			if tt.expectedNames != nil {
				names := make([]string, len(configs))
				for i, c := range configs {
					names[i] = c.Name
				}
				for _, expectedName := range tt.expectedNames {
					found := false
					for _, cfg := range configs {
						if cfg.Name == expectedName {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("Expected name %q not found in %v", expectedName, names)
					}
				}
			}
		})
	}
}

func TestFindDeployConfigs_NoConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	// Don't create config directory

	configs, err := FindDeployConfigs(tmpDir)
	if err != nil {
		t.Fatalf("FindDeployConfigs() error = %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("FindDeployConfigs() returned %d configs, want 0", len(configs))
	}
}

func TestFindDeployConfigs_ServiceExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	content := `service: my-awesome-app
image: myregistry/myapp
`
	if err := os.WriteFile(filepath.Join(configDir, "deploy.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create deploy.yml: %v", err)
	}

	configs, err := FindDeployConfigs(tmpDir)
	if err != nil {
		t.Fatalf("FindDeployConfigs() error = %v", err)
	}

	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}

	if configs[0].Service != "my-awesome-app" {
		t.Errorf("Service = %q, want %q", configs[0].Service, "my-awesome-app")
	}
}

func TestFindDeployConfigs_ServiceFromBaseConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}

	// Base config has the service name
	base := "service: my-awesome-app\nimage: myregistry/myapp\n"
	if err := os.WriteFile(filepath.Join(configDir, "deploy.yml"), []byte(base), 0644); err != nil {
		t.Fatalf("Failed to create deploy.yml: %v", err)
	}
	// Destination file has only overrides, no service key
	staging := "servers:\n  web:\n    - 10.0.0.1\n"
	if err := os.WriteFile(filepath.Join(configDir, "deploy.staging.yml"), []byte(staging), 0644); err != nil {
		t.Fatalf("Failed to create deploy.staging.yml: %v", err)
	}

	configs, err := FindDeployConfigs(tmpDir)
	if err != nil {
		t.Fatalf("FindDeployConfigs() error = %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}
	if configs[0].Name != "staging" {
		t.Errorf("Name = %q, want %q", configs[0].Name, "staging")
	}
	// Service should be inherited from base config
	if configs[0].Service != "my-awesome-app" {
		t.Errorf("Service = %q, want %q (should inherit from base config)", configs[0].Service, "my-awesome-app")
	}
}

func TestSecretsPath(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		dest     *DeployDestination
		expected string
	}{
		{
			name:     "nil destination",
			dest:     nil,
			expected: filepath.Join(tmpDir, ".kamal", "secrets"),
		},
		{
			name:     "production destination",
			dest:     &DeployDestination{Name: "production"},
			expected: filepath.Join(tmpDir, ".kamal", "secrets-production"),
		},
		{
			name:     "staging destination",
			dest:     &DeployDestination{Name: "staging"},
			expected: filepath.Join(tmpDir, ".kamal", "secrets-staging"),
		},
		{
			name:     "development destination",
			dest:     &DeployDestination{Name: "development"},
			expected: filepath.Join(tmpDir, ".kamal", "secrets-development"),
		},
		{
			name:     "empty name destination",
			dest:     &DeployDestination{Name: ""},
			expected: filepath.Join(tmpDir, ".kamal", "secrets"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SecretsPath(tmpDir, tt.dest)
			if result != tt.expected {
				t.Errorf("SecretsPath() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDeployDestination_Label(t *testing.T) {
	tests := []struct {
		dest     DeployDestination
		expected string
	}{
		{
			dest:     DeployDestination{Service: "myapp", Name: ""},
			expected: "myapp",
		},
		{
			dest:     DeployDestination{Service: "myapp", Name: "production"},
			expected: "myapp (production)",
		},
		{
			dest:     DeployDestination{Service: "myapp", Name: "staging"},
			expected: "myapp (staging)",
		},
		{
			dest:     DeployDestination{Service: "web", Name: "development"},
			expected: "web (development)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.dest.Label()
			if result != tt.expected {
				t.Errorf("Label() = %q, want %q", result, tt.expected)
			}
		})
	}
}
