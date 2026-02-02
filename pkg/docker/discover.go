package docker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/shuvro/lazykamal/pkg/ssh"
)

// Container represents a Docker container
type Container struct {
	ID      string
	Name    string
	Image   string
	Status  string
	State   string
	Labels  map[string]string
	Created string
}

// App represents a Kamal-deployed application
type App struct {
	Service     string
	Destination string
	Containers  []Container
	Accessories []Accessory
	ProxyStatus string
}

// Accessory represents a Kamal accessory (redis, postgres, etc.)
type Accessory struct {
	Name       string
	Containers []Container
}

// DiscoverApps discovers all Kamal-deployed apps on a remote server
func DiscoverApps(client *ssh.Client) ([]App, error) {
	// Get all containers with their labels in JSON format
	cmd := `docker ps -a --format '{"ID":"{{.ID}}","Name":"{{.Names}}","Image":"{{.Image}}","Status":"{{.Status}}","State":"{{.State}}","Labels":"{{.Labels}}","Created":"{{.CreatedAt}}"}'`

	output, err := client.Run(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	containers := parseContainers(output)

	// Group containers by service and destination
	apps := groupContainers(containers)

	// Check proxy status for each app
	for i := range apps {
		apps[i].ProxyStatus = checkProxyStatus(client, apps[i].Service)
	}

	// Sort apps by service name
	sort.Slice(apps, func(i, j int) bool {
		if apps[i].Service == apps[j].Service {
			return apps[i].Destination < apps[j].Destination
		}
		return apps[i].Service < apps[j].Service
	})

	return apps, nil
}

// parseContainers parses the docker ps JSON output
func parseContainers(output string) []Container {
	var containers []Container

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var c struct {
			ID      string `json:"ID"`
			Name    string `json:"Name"`
			Image   string `json:"Image"`
			Status  string `json:"Status"`
			State   string `json:"State"`
			Labels  string `json:"Labels"`
			Created string `json:"Created"`
		}

		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}

		container := Container{
			ID:      c.ID,
			Name:    c.Name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Created: c.Created,
			Labels:  parseLabels(c.Labels),
		}

		containers = append(containers, container)
	}

	return containers
}

// parseLabels parses Docker label string into a map
func parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)

	// Docker format: key=value,key2=value2
	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		if idx := strings.Index(pair, "="); idx > 0 {
			key := strings.TrimSpace(pair[:idx])
			value := strings.TrimSpace(pair[idx+1:])
			labels[key] = value
		}
	}

	return labels
}

// groupContainers groups containers into apps by service and destination
func groupContainers(containers []Container) []App {
	// Map: service -> destination -> app
	appMap := make(map[string]map[string]*App)

	for _, c := range containers {
		service := c.Labels["service"]
		if service == "" {
			continue // Not a Kamal container
		}

		destination := c.Labels["destination"]
		if destination == "" {
			destination = "production"
		}

		role := c.Labels["role"]

		// Initialize app map
		if appMap[service] == nil {
			appMap[service] = make(map[string]*App)
		}
		if appMap[service][destination] == nil {
			appMap[service][destination] = &App{
				Service:     service,
				Destination: destination,
			}
		}

		app := appMap[service][destination]

		// Categorize by role
		if role == "web" || role == "" {
			app.Containers = append(app.Containers, c)
		} else {
			// It's an accessory
			found := false
			for i, acc := range app.Accessories {
				if acc.Name == role {
					app.Accessories[i].Containers = append(app.Accessories[i].Containers, c)
					found = true
					break
				}
			}
			if !found {
				app.Accessories = append(app.Accessories, Accessory{
					Name:       role,
					Containers: []Container{c},
				})
			}
		}
	}

	// Flatten to slice
	var apps []App
	for _, destMap := range appMap {
		for _, app := range destMap {
			apps = append(apps, *app)
		}
	}

	return apps
}

// checkProxyStatus checks if kamal-proxy is running for the app
func checkProxyStatus(client *ssh.Client, service string) string {
	// Check if kamal-proxy container is running
	cmd := `docker ps --filter "name=kamal-proxy" --format "{{.Status}}" | head -1`
	output, err := client.Run(cmd)
	if err != nil {
		return "unknown"
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return "not running"
	}

	if strings.Contains(output, "Up") {
		return "running"
	}

	return output
}

// GetContainerLogs gets logs from a container
func GetContainerLogs(client *ssh.Client, containerID string, lines int, follow bool) (string, error) {
	cmd := fmt.Sprintf("docker logs --tail %d", lines)
	if follow {
		cmd += " -f"
	}
	cmd += " " + containerID

	return client.Run(cmd)
}

// StreamContainerLogs streams logs from a container
func StreamContainerLogs(client *ssh.Client, containerID string, onLine func(string), stopCh <-chan struct{}) error {
	cmd := fmt.Sprintf("docker logs -f --tail 100 %s 2>&1", containerID)
	return client.RunStream(cmd, onLine, stopCh)
}

// RestartContainer restarts a container
func RestartContainer(client *ssh.Client, containerID string) error {
	_, err := client.Run(fmt.Sprintf("docker restart %s", containerID))
	return err
}

// StopContainer stops a container
func StopContainer(client *ssh.Client, containerID string) error {
	_, err := client.Run(fmt.Sprintf("docker stop %s", containerID))
	return err
}

// StartContainer starts a container
func StartContainer(client *ssh.Client, containerID string) error {
	_, err := client.Run(fmt.Sprintf("docker start %s", containerID))
	return err
}

// ExecInContainer executes a command in a container
func ExecInContainer(client *ssh.Client, containerID string, command string) (string, error) {
	cmd := fmt.Sprintf("docker exec %s %s", containerID, command)
	return client.Run(cmd)
}

// GetAppVersion gets the current version/image tag of an app
func GetAppVersion(containers []Container) string {
	if len(containers) == 0 {
		return "unknown"
	}

	// Get image tag from first container
	image := containers[0].Image
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		return image[idx+1:]
	}
	return image
}

// CountRunning counts running containers
func CountRunning(containers []Container) int {
	count := 0
	for _, c := range containers {
		if c.State == "running" {
			count++
		}
	}
	return count
}
