package backend

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/posthog/posthog-go"
)

// These variables will be injected at compile time via ldflags
var (
	PostHogAPIKey   = "" // Injected at compile time - no default for security
	PostHogEndpoint = "" // Injected at compile time via ldflags
)

var (
	client posthog.Client
	userID string
)

// Event categories
const (
	CategoryApp           = "app"
	CategoryFirefox       = "firefox"
	CategoryFlow          = "flow"
	CategoryAPICollection = "api_collection"
	CategoryFuzzing       = "fuzzing"
	CategorySettings      = "settings"
	CategoryRequest       = "request"
	CategoryReport        = "report"
	CategoryError         = "error"
)

// InitTelemetry initializes the telemetry client if enabled in settings
func InitTelemetry() {
	// Only initialize client if it hasn't been initialized yet
	if client == nil {
		// Check if telemetry is disabled (no API key provided at compile time)
		if PostHogAPIKey == "" || PostHogAPIKey == "disabled" {
			fmt.Println("Telemetry disabled - no API key provided at compile time")
			return
		}

		config := posthog.Config{
			Endpoint: PostHogEndpoint,
		}

		var err error
		client, err = posthog.NewWithConfig(PostHogAPIKey, config)
		if err != nil {
			return
		}

		// Set user ID if empty
		if gleipSettings.UserID == "" {
			gleipSettings.UserID = uuid.New().String()
			settingsController := NewSettingsController()
			if err := settingsController.UpdateSettings(gleipSettings); err != nil {
				fmt.Printf("Failed to save user ID to settings: %v\n", err)
			}
		}

		// Set the global userID variable
		userID = gleipSettings.UserID

		fmt.Println("Telemetry initialized successfully")
	}
}

// ShutdownTelemetry gracefully shuts down the telemetry client
func ShutdownTelemetry() {
	if client != nil {
		if err := client.Close(); err != nil {
			fmt.Printf("DEBUG: Error closing telemetry client: %v\n", err)
		}
		client = nil
	}
}

// isTelemetryEnabled checks if telemetry is enabled in settings
func isTelemetryEnabled() bool {
	return gleipSettings.TelemetryEnabled
}

// trackEvent is the base function to track events while respecting user settings
func trackEvent(category, action string, properties map[string]interface{}) {
	// Return early if telemetry is disabled
	if !isTelemetryEnabled() || client == nil {
		return
	}

	// Create properties if nil
	if properties == nil {
		properties = make(map[string]interface{})
	}

	// Add category and version to properties
	props := posthog.NewProperties()
	for k, v := range properties {
		props.Set(k, v)
	}
	props.Set("category", category)
	props.Set("version", AppVersion)

	// Build event name
	eventName := fmt.Sprintf("%s:%s", category, action)

	// Debug log
	fmt.Printf("Track: %s, %s\n", eventName, userID)

	// Queue the event
	if err := client.Enqueue(posthog.Capture{
		DistinctId: userID,
		Event:      eventName,
		Properties: props,
	}); err != nil {
		fmt.Printf("Failed to queue telemetry event: %v\n", err)
		return
	}
}

// TrackAppLaunch tracks when the app is launched
func TrackAppLaunch() {
	trackEvent(CategoryApp, "launched", nil)
}

// TrackAppCrash tracks when the app crashes
func TrackAppCrash(err error) {
	props := make(map[string]interface{})
	if err != nil {
		// Only include error type, not the full message which might contain sensitive info
		errorType := fmt.Sprintf("%T", err)
		props["errorType"] = errorType
	}
	trackEvent(CategoryError, "crash", props)
}

// TrackUpdateInstalled tracks when the app is updated
func TrackUpdateInstalled(fromVersion, toVersion string) {
	trackEvent(CategoryApp, "updated", map[string]interface{}{
		"fromVersion": fromVersion,
		"toVersion":   toVersion,
	})
}

// TrackSettingsChanged tracks when settings are changed
func TrackSettingsChanged(settingName string, enabled bool) {
	trackEvent(CategorySettings, "changed", map[string]interface{}{
		"setting": settingName,
		"enabled": enabled,
	})
}

// TrackAPICollectionCreated tracks when a new API collection is created
func TrackAPICollectionCreated(collectionName string) {
	trackEvent(CategoryAPICollection, "created", map[string]interface{}{
		"collectionName": collectionName,
	})
}

// TrackAPICollectionDeleted tracks when a new API collection is deleted
func TrackAPICollectionDeleted(collectionName string) {
	trackEvent(CategoryAPICollection, "deleted", map[string]interface{}{
		"collectionName": collectionName,
	})
}

// TrackAPIRequestImported tracks when a new API request is imported
func TrackAPIRequestImported(collectionName string, requestName string) {
	trackEvent(CategoryAPICollection, "imported", map[string]interface{}{
		"collectionName": collectionName,
		"requestName":    requestName,
	})
}

// TrackFlowCreated tracks when a flow is created
func TrackFlowCreated(flowID string, templateName string) {
	trackEvent(CategoryFlow, "created", map[string]interface{}{
		"flowId":       flowID,
		"templateName": templateName,
	})
}

// TrackFlowExecuted tracks when a flow is executed
func TrackFlowExecuted(flowID string, stepCount int, success bool) {
	trackEvent(CategoryFlow, "executed", map[string]interface{}{
		"flowId":    flowID,
		"stepCount": stepCount,
		"success":   success,
	})
}

// TrackFlowStepExecuted tracks when a step in a flow is executed
func TrackFlowStepExecuted(flowID string, stepType string, success bool) {
	trackEvent(CategoryFlow, "step_executed", map[string]interface{}{
		"flowId":   flowID,
		"stepType": stepType,
		"success":  success,
	})
}

// TrackChefStepExecuted tracks when a chef step is executed
func TrackChefStepExecuted(flowID string, stepID string, actionCount int, success bool) {
	trackEvent(CategoryFlow, "chef_step_executed", map[string]interface{}{
		"flowId":      flowID,
		"stepId":      stepID,
		"actionCount": actionCount,
		"success":     success,
	})
}

// TrackFlowDeleted tracks when a flow is deleted
func TrackFlowDeleted(flowID string) {
	trackEvent(CategoryFlow, "deleted", map[string]interface{}{
		"flowId": flowID,
	})
}

// TrackRequestSent tracks when a request is sent
func TrackRequestSent(method string, statusCode int, fromHistory bool) {
	// Extract just the domain from the URL to avoid sending sensitive paths
	// domain := extractDomain(url)

	trackEvent(CategoryRequest, "sent", map[string]interface{}{
		"method":      method,
		"statusCode":  statusCode,
		"fromHistory": fromHistory,
	})
}

// TrackRequestCopiedToClipboard tracks when a request is copied to clipboard
func TrackRequestCopiedToClipboard(method string, url string, source string) {
	// Extract just the domain from the URL to avoid sending sensitive paths
	// domain := extractDomain(url)

	trackEvent(CategoryRequest, "copied_to_clipboard", map[string]interface{}{
		"method": method,
		// "domain": domain,
		"source": source, // "history" or "api_collection"
	})
}

// TrackRequestCopiedToFlow tracks when a request is copied/pasted into a flow
func TrackRequestCopiedToFlow(method string, source string, flowID string) {
	// Extract just the domain from the URL to avoid sending sensitive paths
	// domain := extractDomain(url)

	trackEvent(CategoryRequest, "copied_to_flow", map[string]interface{}{
		"method": method,
		"source": source, // "history" or "api_collection"
		"flowId": flowID,
	})
}

// TrackFuzzingStarted tracks when fuzzing begins
func TrackFuzzingStarted(flowID string, stepID string, wordlistSize int) {
	trackEvent(CategoryFuzzing, "started", map[string]interface{}{
		"flowId":       flowID,
		"stepId":       stepID,
		"wordlistSize": wordlistSize,
	})
}

// TrackFuzzingCompleted tracks when fuzzing completes
func TrackFuzzingCompleted(flowID string, stepID string, resultsCount int, cancelled bool) {
	trackEvent(CategoryFuzzing, "completed", map[string]interface{}{
		"flowId":       flowID,
		"stepId":       stepID,
		"resultsCount": resultsCount,
		"cancelled":    cancelled,
	})
}

// TrackReportGenerated tracks when a report is generated
func TrackReportGenerated(format string, flowCount int) {
	trackEvent(CategoryReport, "generated", map[string]interface{}{
		"format":    format,
		"flowCount": flowCount,
	})
}

// TrackFirefoxAction tracks Firefox-related actions
func TrackFirefoxAction(action string, success bool) {
	props := map[string]interface{}{
		"success": success,
		"os":      strings.ToLower(GetOSInfo()),
	}
	trackEvent(CategoryFirefox, action, props)
}

// GetOSInfo returns the operating system information
func GetOSInfo() string {
	return strings.ToLower(SystemInfo.OS)
}

// TrackError tracks errors that occur in the application
func TrackError(component string, errorType string) {
	props := map[string]interface{}{
		"component": component,
		"errorType": errorType,
	}
	trackEvent(CategoryError, "occurred", props)
}

// SystemInfo contains system information
var SystemInfo struct {
	OS      string
	Version string
	Arch    string
}

// InitSystemInfo initializes system information
func InitSystemInfo() {
	SystemInfo.OS = "unknown"
	if runtime, ok := gleipSettings.RuntimeInfo["os"]; ok {
		SystemInfo.OS = runtime
	}

	SystemInfo.Arch = "unknown"
	if arch, ok := gleipSettings.RuntimeInfo["arch"]; ok {
		SystemInfo.Arch = arch
	}

	SystemInfo.Version = AppVersion
}
