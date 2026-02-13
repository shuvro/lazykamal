package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	repoOwner  = "shuvro"
	repoName   = "lazykamal"
	binaryName = "lazykamal"
)

// Release represents a GitHub release
type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// GetLatestVersion fetches the latest version from GitHub
func GetLatestVersion() (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", repoOwner, repoName)

	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("failed to check for updates: HTTP %d", resp.StatusCode)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	return release.TagName, nil
}

// NeedsUpdate compares current version with latest using numeric semver comparison.
func NeedsUpdate(current, latest string) bool {
	current = strings.TrimPrefix(current, "v")
	latest = strings.TrimPrefix(latest, "v")
	if current == "dev" {
		return false
	}
	cParts := strings.Split(current, ".")
	lParts := strings.Split(latest, ".")
	for i := 0; i < len(cParts) && i < len(lParts); i++ {
		c, _ := strconv.Atoi(cParts[i])
		l, _ := strconv.Atoi(lParts[i])
		if c < l {
			return true
		}
		if c > l {
			return false
		}
	}
	return false
}

// getAssetName returns the expected asset name for the current platform
func getAssetName(version string) string {
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Version without 'v' prefix
	ver := strings.TrimPrefix(version, "v")

	ext := "tar.gz"
	if os == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("%s_%s_%s_%s.%s", binaryName, ver, os, arch, ext)
}

// getDownloadURL returns the download URL for the current platform
func getDownloadURL(version string) string {
	assetName := getAssetName(version)
	return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s",
		repoOwner, repoName, version, assetName)
}

// DoUpgrade performs the self-upgrade
func DoUpgrade(currentVersion string) error {
	fmt.Println("Checking for updates...")

	latestVersion, err := GetLatestVersion()
	if err != nil {
		return err
	}

	if !NeedsUpdate(currentVersion, latestVersion) {
		fmt.Printf("Already at latest version (%s)\n", currentVersion)
		return nil
	}

	fmt.Printf("Upgrading from %s to %s...\n", currentVersion, latestVersion)

	// Get current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	// Download new version
	downloadURL := getDownloadURL(latestVersion)
	fmt.Printf("Downloading %s...\n", getAssetName(latestVersion))

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("failed to download: HTTP %d (asset may not exist for your platform)", resp.StatusCode)
	}

	// Create temp file
	tmpDir, err := os.MkdirTemp("", "lazykamal-upgrade")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Extract binary from tar.gz
	fmt.Println("Extracting...")
	newBinaryPath := filepath.Join(tmpDir, binaryName)

	if runtime.GOOS == "windows" {
		return fmt.Errorf("self-upgrade on Windows is not supported. Please use: scoop update lazykamal")
	}

	if err := extractTarGz(resp.Body, tmpDir, binaryName); err != nil {
		return fmt.Errorf("failed to extract: %w", err)
	}

	// Check if we need elevated permissions
	if err := checkWritePermission(execPath); err != nil {
		fmt.Println("\nPermission denied. Try running with sudo:")
		fmt.Printf("  sudo %s --upgrade\n", execPath)
		return err
	}

	// Replace current binary
	fmt.Println("Installing...")

	// Rename old binary as backup
	backupPath := execPath + ".bak"
	if err := os.Rename(execPath, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Move new binary
	if err := copyFile(newBinaryPath, execPath); err != nil {
		// Restore backup on failure
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(execPath, 0755); err != nil {
		_ = os.Rename(backupPath, execPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Remove backup
	_ = os.Remove(backupPath)

	fmt.Printf("\n✓ Successfully upgraded to %s\n", latestVersion)
	return nil
}

func extractTarGz(r io.Reader, destDir, targetFile string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// Only extract the target file
		if header.Name == targetFile || filepath.Base(header.Name) == targetFile {
			if err := extractFile(tr, filepath.Join(destDir, targetFile)); err != nil {
				return err
			}
			return nil
		}
	}

	return fmt.Errorf("binary not found in archive")
}

func extractFile(r io.Reader, destPath string) error {
	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, r)
	return err
}

func checkWritePermission(path string) error {
	dir := filepath.Dir(path)
	testFile := filepath.Join(dir, ".lazykamal-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Remove(testFile)
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
