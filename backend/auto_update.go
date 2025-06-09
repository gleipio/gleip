package backend

import (
	"Gleip/backend/paths"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	rt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GitHub API URL for fetching latest release
const (
	githubRepoOwner = "gleipio"
	githubRepoName  = "gleip"
)

// GitHubRelease represents the response from GitHub API
type GitHubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		BrowserDownloadURL string `json:"browser_download_url"`
		Name               string `json:"name"`
		ContentType        string `json:"content_type"`
		Size               int    `json:"size"`
	} `json:"assets"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes"`
	DownloadURL    string `json:"downloadUrl"`
	IsUpdateNeeded bool   `json:"isUpdateNeeded"`
}

// CheckForUpdates checks if a newer version is available on GitHub
func (a *App) CheckForUpdates() (*UpdateInfo, error) {
	// Use the GitHub API to get the latest release
	apiURL := fmt.Sprintf(paths.GlobalURLs.Services.GitHubAPI, githubRepoOwner, githubRepoName)

	// Create a client with a timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Make the request
	resp, err := client.Get(apiURL)
	if err != nil {
		TrackError("auto_update", "api_request_failed")
		return nil, fmt.Errorf("failed to check for updates: %v", err)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		TrackError("auto_update", fmt.Sprintf("api_status_error_%d", resp.StatusCode))
		return nil, fmt.Errorf("failed to check for updates: API returned status %d", resp.StatusCode)
	}

	// Parse the response
	var release GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		TrackError("auto_update", "json_parse_error")
		return nil, fmt.Errorf("failed to parse update information: %v", err)
	}

	// Clean up the version strings (remove 'v' prefix if present)
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimSuffix(AppVersion, "-alpha") // Remove any -alpha suffix
	currentVersion = strings.TrimPrefix(currentVersion, "v")

	// Find the macOS asset
	var downloadURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".dmg") && strings.Contains(strings.ToLower(asset.Name), "mac") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	// Check if an update is needed
	isUpdateNeeded := CompareVersions(currentVersion, latestVersion) < 0

	// Create update info
	updateInfo := &UpdateInfo{
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
		ReleaseNotes:   release.Body,
		DownloadURL:    downloadURL,
		IsUpdateNeeded: isUpdateNeeded,
	}

	return updateInfo, nil
}

// CompareVersions compares version strings in the format YYYY.MM.DD
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func CompareVersions(v1, v2 string) int {
	// Handle minor release format (e.g., 2025.06.08-1)
	v1Parts := strings.Split(v1, "-")
	v2Parts := strings.Split(v2, "-")

	// Compare base versions first
	baseV1 := v1Parts[0]
	baseV2 := v2Parts[0]

	// Split versions into parts
	parts1 := strings.Split(baseV1, ".")
	parts2 := strings.Split(baseV2, ".")

	// Compare each part
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		num1, err1 := strconv.Atoi(parts1[i])
		num2, err2 := strconv.Atoi(parts2[i])

		// If we can't parse either part, fallback to string comparison
		if err1 != nil || err2 != nil {
			if parts1[i] < parts2[i] {
				return -1
			} else if parts1[i] > parts2[i] {
				return 1
			}
			continue
		}

		// Compare numeric values
		if num1 < num2 {
			return -1
		} else if num1 > num2 {
			return 1
		}
	}

	// If all parts match but one version has more parts
	if len(parts1) < len(parts2) {
		return -1
	} else if len(parts1) > len(parts2) {
		return 1
	}

	// If base versions are equal, check minor release suffix
	if len(v1Parts) == 1 && len(v2Parts) > 1 {
		// v1 has no minor version but v2 does (e.g., 2025.06.08 vs 2025.06.08-1)
		return -1
	} else if len(v1Parts) > 1 && len(v2Parts) == 1 {
		// v1 has a minor version but v2 doesn't
		return 1
	} else if len(v1Parts) > 1 && len(v2Parts) > 1 {
		// Both have minor versions, compare them
		minor1, err1 := strconv.Atoi(v1Parts[1])
		minor2, err2 := strconv.Atoi(v2Parts[1])

		if err1 != nil || err2 != nil {
			// If we can't parse as numbers, fall back to string comparison
			if v1Parts[1] < v2Parts[1] {
				return -1
			} else if v1Parts[1] > v2Parts[1] {
				return 1
			}
		} else {
			// Compare as numbers
			if minor1 < minor2 {
				return -1
			} else if minor1 > minor2 {
				return 1
			}
		}
	}

	// Versions are equal
	return 0
}

// DownloadAndInstallUpdate downloads the update, mounts the DMG, and launches it
func (a *App) DownloadAndInstallUpdate(ctx context.Context, downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("download URL is empty")
	}

	// Create a temporary directory for the download
	tmpDir, err := os.MkdirTemp("", "gleip-update-*")
	if err != nil {
		TrackError("auto_update", "temp_dir_creation_failed")
		return fmt.Errorf("failed to create temporary directory: %v", err)
	}

	// Create the dmg file path
	dmgPath := filepath.Join(tmpDir, "update.dmg")

	// Notify the frontend that we're downloading
	rt.EventsEmit(ctx, "update:downloading", map[string]interface{}{
		"status": "downloading",
	})

	// Download the file
	err = downloadFile(downloadURL, dmgPath)
	if err != nil {
		TrackError("auto_update", "download_failed")
		return fmt.Errorf("failed to download update: %v", err)
	}

	// Notify the frontend that we're mounting
	rt.EventsEmit(ctx, "update:extracting", map[string]interface{}{
		"status": "mounting",
	})

	// Mount the DMG file
	mountPoint, err := mountDMG(dmgPath)
	if err != nil {
		TrackError("auto_update", "mount_failed")
		return fmt.Errorf("failed to mount DMG: %v", err)
	}

	// Find the .app bundle in the mounted volume
	appPath, err := findAppBundle(mountPoint)
	if err != nil {
		// Try to unmount before returning the error
		_ = unmountDMG(mountPoint)
		TrackError("auto_update", "app_bundle_not_found")
		return fmt.Errorf("failed to find application bundle: %v", err)
	}

	// Notify the frontend that we're installing
	rt.EventsEmit(ctx, "update:installing", map[string]interface{}{
		"status": "installing",
	})

	// Copy the app to Applications folder
	destinationPath := "/Applications/Gleip.app"
	err = copyApp(appPath, destinationPath)
	if err != nil {
		// Try to unmount before returning the error
		_ = unmountDMG(mountPoint)
		TrackError("auto_update", "copy_failed")
		return fmt.Errorf("failed to install application: %v", err)
	}

	// Unmount the DMG
	err = unmountDMG(mountPoint)
	if err != nil {
		// Not fatal, just log it
		fmt.Printf("Warning: Failed to unmount DMG: %v\n", err)
	}

	// Notify the frontend that we're launching
	rt.EventsEmit(ctx, "update:launching", map[string]interface{}{
		"status": "launching",
	})

	// Track successful update
	latestVersion := extractVersionFromPath(destinationPath)
	TrackUpdateInstalled(AppVersion, latestVersion)

	// Notify the frontend that we're done and will quit
	rt.EventsEmit(ctx, "update:complete", map[string]interface{}{
		"status": "complete",
	})

	// Create update flag file in app support directory to indicate an update occurred
	updateFlagPath := createUpdateFlagFile()
	if updateFlagPath != "" {
		fmt.Printf("Created update flag file at: %s\n", updateFlagPath)
	}

	// Create a command that will sleep to ensure our app has time to exit
	// then launch the new app instance with a clean start (using -n flag)
	launchScript := filepath.Join(tmpDir, "update_launcher.sh")
	scriptContent := fmt.Sprintf(`#!/bin/bash

# Sleep to make sure the original app is closed
sleep 1

# Force a new instance of the app (-n flag)
/usr/bin/open -n "%s"

# Clean up temporary files
rm -rf "%s"
`, destinationPath, tmpDir)

	if err := os.WriteFile(launchScript, []byte(scriptContent), 0755); err != nil {
		fmt.Printf("Warning: Failed to create launch script: %v\n", err)

		// Fall back to simple exit - user will have to manually start the app
		go func() {
			time.Sleep(500 * time.Millisecond)
			os.Exit(0)
		}()
	} else {
		// Execute the launch script as a detached process
		cmd := exec.Command("/bin/bash", launchScript)
		setProcAttributes(cmd)

		if err := cmd.Start(); err != nil {
			fmt.Printf("Warning: Failed to execute launch script: %v\n", err)

			// Fall back to simple exit
			go func() {
				time.Sleep(500 * time.Millisecond)
				os.Exit(0)
			}()
		} else {
			fmt.Printf("Launch script started successfully, exiting now\n")
			go func() {
				time.Sleep(200 * time.Millisecond)
				os.Exit(0)
			}()
		}
	}

	// Small delay to allow events to be emitted before potential exit
	time.Sleep(300 * time.Millisecond)

	return nil
}

// createUpdateFlagFile creates a flag file to indicate an update has occurred
// This flag can be checked on application startup to handle post-update tasks
func createUpdateFlagFile() string {
	flagPath := getUpdateFlagFilePath()
	if flagPath == "" {
		return ""
	}

	// Write the current time to the flag file
	content := fmt.Sprintf("Update completed at: %s\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(flagPath, []byte(content), 0644); err != nil {
		fmt.Printf("Warning: Failed to create update flag file: %v\n", err)
		return ""
	}

	return flagPath
}

// downloadFile downloads a file from a URL to a local path
func downloadFile(url, filepath string) error {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

// mountDMG mounts a DMG file and returns the mount point
func mountDMG(dmgPath string) (string, error) {
	// Use hdiutil to mount the DMG
	cmd := exec.Command("hdiutil", "attach", dmgPath, "-nobrowse", "-mountrandom", "/tmp")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to mount DMG: %v, output: %s", err, output)
	}

	// Parse the output to get the mount point
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "/tmp/") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				// The mount point is the last field
				return fields[len(fields)-1], nil
			}
		}
	}

	return "", fmt.Errorf("could not determine mount point from hdiutil output")
}

// unmountDMG unmounts a DMG file
func unmountDMG(mountPoint string) error {
	cmd := exec.Command("hdiutil", "detach", mountPoint)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to unmount DMG: %v, output: %s", err, output)
	}
	return nil
}

// copyApp copies an app bundle to a destination path
func copyApp(sourcePath, destPath string) error {
	// Remove existing app if it exists
	if _, err := os.Stat(destPath); err == nil {
		// If the app exists, try to remove it
		if err := os.RemoveAll(destPath); err != nil {
			return fmt.Errorf("failed to remove existing app: %v", err)
		}
	}

	// Use ditto to preserve bundle structure and permissions
	cmd := exec.Command("ditto", sourcePath, destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to copy app: %v, output: %s", err, output)
	}
	return nil
}

// findAppBundle finds an .app bundle in the given directory
func findAppBundle(dirPath string) (string, error) {
	var appPath string

	// Walk the directory looking for .app bundles
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && strings.HasSuffix(path, ".app") {
			appPath = path
			return filepath.SkipDir // Stop walking once we find an .app
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if appPath == "" {
		return "", fmt.Errorf("no .app bundle found in %s", dirPath)
	}

	return appPath, nil
}

// extractVersionFromPath extracts version from app path
// This is a fallback if we can't determine the version otherwise
func extractVersionFromPath(appPath string) string {
	// Default to "unknown" if we can't extract
	version := "unknown"

	// Try to extract from path name (this is very dependent on naming convention)
	baseName := filepath.Base(appPath)
	if strings.HasSuffix(baseName, ".app") {
		nameWithoutExt := strings.TrimSuffix(baseName, ".app")
		parts := strings.Split(nameWithoutExt, "-")
		if len(parts) > 1 {
			// Assuming format like "Gleip-2025.05.20.app"
			version = parts[len(parts)-1]
		}
	}

	return version
}

// InitAutoUpdateCheck performs an update check at startup
func (a *App) InitAutoUpdateCheck() {
	// Perform the check in a goroutine to not block startup
	go func() {
		// Wait a few seconds to allow the app to fully start
		time.Sleep(3 * time.Second)

		updateInfo, err := a.CheckForUpdates()
		if err != nil {
			fmt.Printf("Failed to check for updates: %v\n", err)
			return
		}

		// If an update is available, notify the frontend
		if updateInfo.IsUpdateNeeded {
			rt.EventsEmit(a.ctx, "update:available", updateInfo)
		}
	}()
}

// getUpdateFlagFilePath returns the path to the update flag file
func getUpdateFlagFilePath() string {
	// Get the app support directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("Warning: Failed to get user home directory: %v\n", err)
		return ""
	}

	// Use platform-specific app data directory
	var appDataDir string
	switch runtime.GOOS {
	case "darwin":
		appDataDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip")
	case "linux":
		appDataDir = filepath.Join(homeDir, ".config", "Gleip")
	case "windows":
		appDataDir = filepath.Join(homeDir, "AppData", "Local", "Gleip")
	default:
		appDataDir = filepath.Join(homeDir, ".gleip") // Fallback
	}

	// Ensure directory exists
	if err := os.MkdirAll(appDataDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create app data directory: %v\n", err)
		return ""
	}

	return filepath.Join(appDataDir, "update_completed")
}
