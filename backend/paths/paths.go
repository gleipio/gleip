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
	ScanTargetsDir    string // Scan targets
	ScanConfigsDir    string // Scan configurations
	ScanResultsDir    string // Scan results
	BrowsersDir       string // Browser installations
	FirefoxDir        string // Firefox installation
	FirefoxProfileDir string // Firefox profile
	CertificatesDir   string // SSL certificates
}

var GlobalPaths AppPaths

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
	GlobalPaths.ScanTargetsDir = filepath.Join(GlobalPaths.AppDataDir, "scan_targets")
	GlobalPaths.ScanConfigsDir = filepath.Join(GlobalPaths.AppDataDir, "scan_configs")
	GlobalPaths.ScanResultsDir = filepath.Join(GlobalPaths.AppDataDir, "scan_results")
	GlobalPaths.BrowsersDir = filepath.Join(GlobalPaths.AppDataDir, "browsers")
	GlobalPaths.FirefoxDir = filepath.Join(GlobalPaths.BrowsersDir, "firefox")
	GlobalPaths.FirefoxProfileDir = filepath.Join(GlobalPaths.FirefoxDir, "profile")
	GlobalPaths.CertificatesDir = filepath.Join(GlobalPaths.AppDataDir, "ca")

	// Create all directories except for ProjectsDir which is created only when needed
	dirsToCreate := []string{
		GlobalPaths.AppDataDir,
		// ProjectsDir is no longer created here
		GlobalPaths.TempDir,
		GlobalPaths.ScanTargetsDir,
		GlobalPaths.ScanConfigsDir,
		GlobalPaths.ScanResultsDir,
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
