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
// In Kamal, deploy.yml is the base config and deploy.<destination>.yml files are destination
// overlays. When destination files exist, only those are returned (deploy.yml is the shared
// base, not a separate destination). When no destination files exist, deploy.yml is returned
// as a single entry with an empty destination name.
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
	var baseConfig *DeployDestination
	var destinations []DeployDestination
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		configPath := filepath.Join(configDir, name)
		if name == "deploy.yml" || name == "deploy.yaml" {
			// This is the base config file. Only used as a destination entry
			// when no destination-specific files exist.
			data, err := os.ReadFile(configPath)
			if err != nil {
				continue
			}
			var cfg map[string]interface{}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				continue
			}
			service := "default"
			if s, ok := cfg["service"].(string); ok && s != "" {
				service = s
			}
			baseConfig = &DeployDestination{
				Name:       "",
				ConfigPath: configPath,
				Service:    service,
				Config:     cfg,
			}
		} else if strings.HasPrefix(name, "deploy.") && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml")) {
			ext := name[strings.LastIndex(name, "."):]
			destName := name[7 : len(name)-len(ext)]
			data, err := os.ReadFile(configPath)
			if err != nil {
				continue
			}
			var cfg map[string]interface{}
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				continue
			}
			// For destination files, read service from the base config if not specified
			// in the destination file, falling back to destination name.
			service := destName
			if s, ok := cfg["service"].(string); ok && s != "" {
				service = s
			}
			destinations = append(destinations, DeployDestination{
				Name:       destName,
				ConfigPath: configPath,
				Service:    service,
				Config:     cfg,
			})
		}
	}
	// If destination files exist, return only those.
	// deploy.yml is the base config shared by all destinations, not a separate target.
	if len(destinations) > 0 {
		// Try to fill in service name from base config for destinations that
		// don't define their own service (common since destination files only
		// contain overrides).
		if baseConfig != nil {
			for i := range destinations {
				if _, ok := destinations[i].Config["service"].(string); !ok {
					destinations[i].Service = baseConfig.Service
				}
			}
		}
		return destinations, nil
	}
	// No destination files: deploy.yml is the single target (no -d flag needed).
	if baseConfig != nil {
		return []DeployDestination{*baseConfig}, nil
	}
	return nil, nil
}

// SecretsPath returns the path to the secrets file for the given destination.
// Kamal uses .kamal/secrets for the base (no destination) and .kamal/secrets-<destination>
// for named destinations. Returns the destination-specific path regardless of whether
// the file exists (to support file creation).
func SecretsPath(dir string, dest *DeployDestination) string {
	base := filepath.Join(dir, ".kamal", "secrets")
	if dest != nil && dest.Name != "" {
		return base + "-" + dest.Name
	}
	return base
}
