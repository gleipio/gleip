package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// AppPaths contains all application paths
type AppPaths struct {
	// Base directories
	HomeDir     string // User's home directory
	AppDataDir  string // Main application data directory
	ProjectsDir string // Projects directory

	// Subdirectories
	TempDir           string // Temporary files
	BrowsersDir       string // Browser installations
	FirefoxDir        string // Firefox installation
	FirefoxProfileDir string // Firefox profile
	CertificatesDir   string // SSL certificates
}

// URLs contains all external service URLs
type URLs struct {
	Firefox  FirefoxURLs
	Services ServiceURLs
}

// ServiceURLs contains URLs for various external services
type ServiceURLs struct {
	PostHogEndpoint string // PostHog telemetry endpoint
	GitHubAPI       string // GitHub API base URL for releases
}

// FirefoxURLs contains Firefox download URLs for different platforms
type FirefoxURLs struct {
	MacOS   string // macOS Firefox download URL
	Linux   string // Linux Firefox download URL
	Windows string // Windows Firefox download URL
}

var GlobalPaths AppPaths
var GlobalURLs URLs

// InitPaths initializes all application paths
func InitPaths() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home dir cannot be determined
		homeDir = "."
	}

	GlobalPaths.HomeDir = homeDir

	// Projects directory is always in Documents
	GlobalPaths.ProjectsDir = filepath.Join(homeDir, "Documents", "GleipProjects")

	// Set platform-specific app data directory
	switch runtime.GOOS {
	case "darwin":
		GlobalPaths.AppDataDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip")
	case "linux":
		GlobalPaths.AppDataDir = filepath.Join(homeDir, ".config", "Gleip")
	case "windows":
		GlobalPaths.AppDataDir = filepath.Join(homeDir, "AppData", "Local", "Gleip")
	default:
		GlobalPaths.AppDataDir = filepath.Join(homeDir, ".gleip") // Fallback
	}

	// Set subdirectories
	GlobalPaths.TempDir = filepath.Join(GlobalPaths.AppDataDir, "temp")
	GlobalPaths.BrowsersDir = filepath.Join(GlobalPaths.AppDataDir, "browsers")
	GlobalPaths.FirefoxDir = filepath.Join(GlobalPaths.BrowsersDir, "firefox")
	GlobalPaths.FirefoxProfileDir = filepath.Join(GlobalPaths.FirefoxDir, "profile")
	GlobalPaths.CertificatesDir = filepath.Join(GlobalPaths.AppDataDir, "ca")

	// Initialize URLs
	GlobalURLs = URLs{
		Firefox: FirefoxURLs{
			MacOS:   "https://download.mozilla.org/?product=firefox-latest&os=osx&lang=en-US",
			Linux:   "https://download.mozilla.org/?product=firefox-latest&os=linux64&lang=en-US",
			Windows: "https://download.mozilla.org/?product=firefox-latest&os=win64&lang=en-US",
		},
		Services: ServiceURLs{
			PostHogEndpoint: "https://eu.i.posthog.com",
			GitHubAPI:       "https://api.github.com/repos/%s/%s/releases/latest",
		},
	}

	// Create all directories except for ProjectsDir which is created only when needed
	dirsToCreate := []string{
		GlobalPaths.AppDataDir,
		// ProjectsDir is no longer created here
		GlobalPaths.TempDir,
		GlobalPaths.BrowsersDir,
		GlobalPaths.FirefoxDir,
		GlobalPaths.FirefoxProfileDir,
		GlobalPaths.CertificatesDir,
	}

	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// EnsureProjectsDir creates the projects directory if it doesn't exist yet
// This should be called before saving a project to disk
func EnsureProjectsDir() error {
	if GlobalPaths.ProjectsDir == "" {
		return fmt.Errorf("projects directory path is not set")
	}

	return os.MkdirAll(GlobalPaths.ProjectsDir, 0755)
}

// GetTempFilePath returns a path for a temporary file with the given name
func GetTempFilePath(filename string) string {
	return filepath.Join(GlobalPaths.TempDir, filename)
}

// GetProjectPath returns the path for a project file with the given ID
func GetProjectPath(projectID string) string {
	return filepath.Join(GlobalPaths.ProjectsDir, projectID+".gleip")
}

// GetFirefoxDownloadURL returns the Firefox download URL for the current platform
func GetFirefoxDownloadURL() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return GlobalURLs.Firefox.MacOS, nil
	case "linux":
		return GlobalURLs.Firefox.Linux, nil
	case "windows":
		return GlobalURLs.Firefox.Windows, nil
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}
