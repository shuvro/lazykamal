package kamal

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DeployDestination represents a Kamal deploy target (config/deploy.yml or config/deploy.<name>.yml).
type DeployDestination struct {
	Name       string
	ConfigPath string
	Service    string
	Config     map[string]interface{}
}

// FindDeployConfigs discovers config/deploy*.yml and config/deploy*.yaml in the given directory.
func FindDeployConfigs(dir string) ([]DeployDestination, error) {
	configDir := filepath.Join(dir, "config")
	fi, err := os.Stat(configDir)
	if err != nil || !fi.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil, err
	}
	var out []DeployDestination
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		var destName string
		if name == "deploy.yml" || name == "deploy.yaml" {
			destName = "production"
		} else if strings.HasPrefix(name, "deploy.") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")) {
			ext := name[strings.LastIndex(name, "."):]
			destName = name[7 : len(name)-len(ext)]
		} else {
			continue
		}
		configPath := filepath.Join(configDir, name)
		data, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var cfg map[string]interface{}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		service := destName
		if s, ok := cfg["service"].(string); ok && s != "" {
			service = s
		}
		out = append(out, DeployDestination{
			Name:       destName,
			ConfigPath: configPath,
			Service:    service,
			Config:     cfg,
		})
	}
	return out, nil
}

// SecretsPath returns the path to the secrets file for the given destination.
// Kamal uses .kamal/secrets by default and .kamal/secrets.<destination> for non-production.
// For non-production destinations, it returns the destination-specific path regardless of
// whether the file exists (to support file creation).
func SecretsPath(dir string, dest *DeployDestination) string {
	base := filepath.Join(dir, ".kamal", "secrets")
	if dest != nil && dest.Name != "" && dest.Name != "production" {
		return base + "-" + dest.Name
	}
	return base
}
