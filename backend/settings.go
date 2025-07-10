package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var telemetryEnabled = true
var settingsFile = "settings.json"

type GleipSettings struct {
	TelemetryEnabled bool              `json:"telemetryEnabled"`
	UserID           string            `json:"userId,omitempty"`      // Persistent anonymous user ID
	RuntimeInfo      map[string]string `json:"runtimeInfo,omitempty"` // System information
}

var gleipSettings GleipSettings

// GetGleipAppDir returns the platform-specific Gleip application directory
func GetGleipAppDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	var settingsDir string
	// Get platform-specific directory
	switch runtime.GOOS {
	case "linux":
		settingsDir = filepath.Join(homeDir, ".config", "Gleip")
	case "darwin":
		settingsDir = filepath.Join(homeDir, "Library", "Application Support", "Gleip")
	case "windows":
		settingsDir = filepath.Join(homeDir, "AppData", "Local", "Gleip")
	default:
		return "", fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	return settingsDir, nil
}

// SettingsController handles settings operations
type SettingsController struct{}

// NewSettingsController creates a new settings controller
func NewSettingsController() *SettingsController {
	return &SettingsController{}
}

// GetSettings returns the current settings
func (sc *SettingsController) GetSettings() GleipSettings {
	return gleipSettings
}

// UpdateSettings updates the settings and saves to file
func (sc *SettingsController) UpdateSettings(settings GleipSettings) error {
	// Track the setting change
	if settings.TelemetryEnabled != gleipSettings.TelemetryEnabled {
		TrackSettingsChanged("telemetryEnabled", settings.TelemetryEnabled)
	}

	// Update the settings
	gleipSettings = settings

	// Preserve the UserID if it's being cleared for some reason
	if gleipSettings.UserID == "" && settings.UserID != "" {
		gleipSettings.UserID = settings.UserID
	}

	file, err := json.Marshal(gleipSettings)
	if err != nil {
		// Track error
		TrackError("settings", "json_marshal")
		return err
	}

	// Get the settings directory
	settingsDir, err := GetGleipAppDir()
	if err != nil {
		TrackError("settings", "directory_error")
		return err
	}

	// Ensure the directory exists
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		TrackError("settings", "directory_create")
		return err
	}

	// Write settings to file
	settingsPath := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsPath, file, 0644); err != nil {
		// Track error
		TrackError("settings", "file_write")
		return err
	}

	return nil
}

// InitSettings initializes the settings system
func InitSettings() error {
	// Get the settings directory
	settingsDir, err := GetGleipAppDir()
	if err != nil {
		return err
	}

	// Create settings directory if it doesn't exist
	if err := os.MkdirAll(settingsDir, 0755); err != nil {
		return err
	}

	// Set the settings file path
	settingsFile = filepath.Join(settingsDir, "settings.json")

	// Check if settings file exists
	if _, err := os.Stat(settingsFile); os.IsNotExist(err) {
		// Initialize runtime info
		runtimeInfo := map[string]string{
			"os":   runtime.GOOS,
			"arch": runtime.GOARCH,
		}

		gleipSettings = GleipSettings{
			TelemetryEnabled: telemetryEnabled,
			UserID:           "", // Will be set in InitTelemetry if empty
			RuntimeInfo:      runtimeInfo,
		}
		file, err := json.Marshal(gleipSettings)
		if err != nil {
			return err
		}

		if err := os.WriteFile(settingsFile, file, 0644); err != nil {
			return err
		}

		return nil
	} else {
		file, err := os.ReadFile(settingsFile)
		if err != nil {
			return err
		}

		if err := json.Unmarshal(file, &gleipSettings); err != nil {
			if gleipSettings.RuntimeInfo == nil {
				// Initialize runtime info
				gleipSettings.RuntimeInfo = map[string]string{
					"os":   runtime.GOOS,
					"arch": runtime.GOARCH,
				}
			}

			// Save the updated settings
			updatedFile, _ := json.Marshal(gleipSettings)
			os.WriteFile(settingsFile, updatedFile, 0644)
		}

		// Initialize SystemInfo from settings
		InitSystemInfo()

		return nil
	}
}
