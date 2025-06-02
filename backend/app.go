package backend

import (
	"Gleip/backend/network"
	"Gleip/backend/paths"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"Gleip/backend/cert"

	"github.com/google/uuid"
	rt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// AppVersion is injected at build time via -ldflags
var AppVersion = "dev" // fallback for development

const defaultProjectName = "New Project"

// Project represents a complete Gleip project with all UI state and data
type Project struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	FilePathOnDisk string `json:"filePathOnDisk,omitempty"` // Actual path where the project file is/should be saved

	// Request History Tab data
	SearchQuery     string                     `json:"searchQuery,omitempty"`
	RequestHistory  []*network.HTTPTransaction `json:"requestHistory,omitempty"` // Changed to slice of pointers
	ActiveRequestID string                     `json:"activeRequestId,omitempty"`

	// Request GleipFlow Tab data
	GleipFlows          []*GleipFlow                 `json:"gleipFlows,omitempty"`
	Variables           map[string]string            `json:"variables,omitempty"`
	SelectedGleipFlowID string                       `json:"selectedGleipFlowId,omitempty"`
	ExecutionStates     map[string][]ExecutionResult `json:"executionStates,omitempty"`

	// API Collections data
	APICollections        []APICollection `json:"apiCollections,omitempty"`
	SelectedAPICollection string          `json:"selectedApiCollectionId,omitempty"`

	// Other application state
	ProxyEnabled bool          `json:"proxyEnabled"`
	ProxyPort    int           `json:"proxyPort"`
	ScanTargets  []*ScanTarget `json:"scanTargets,omitempty"`
	ScanConfigs  []*ScanConfig `json:"scanConfigs,omitempty"`
	ScanResults  []*ScanResult `json:"scanResults,omitempty"`
}

// App struct
type App struct {
	ctx                 context.Context
	proxyServer         *ProxyServer
	gleipFlowsMutex     sync.RWMutex
	gleipFlowsCache     map[string]*GleipFlow
	currentProject      *Project
	projectMutex        sync.RWMutex
	apiCollectionsMutex sync.RWMutex
	scanTargetsMutex    sync.RWMutex
	scanConfigsMutex    sync.RWMutex
	scanResultsMutex    sync.RWMutex
	scanTargetsCache    map[string]*ScanTarget
	scanConfigsCache    map[string]*ScanConfig
	scanResultsCache    map[string]*ScanResult
	activeScans         map[string]*ScanResult
	activeScansMutex    sync.RWMutex
	firefoxPID          int
	gleipFlowExecutor   *GleipFlowExecutor
	tempProjectPath     string // Path to temporary project file

	// Auto-save related fields
	autoSaveTimer      *time.Timer
	autoSaveMutex      sync.Mutex
	autoSaveRequested  bool
	autoSaveInProgress bool
	autoSaveInterval   time.Duration // Minimum time between auto-saves

	// Dirty state tracking for incremental saves
	dirtyStateMutex     sync.RWMutex
	dirtyRequestHistory bool
	dirtyGleipFlows     bool
	dirtyAPICollections bool
	dirtyScanTargets    bool
	dirtyScanConfigs    bool
	dirtyScanResults    bool
	dirtyProjectMeta    bool // For project name, settings, etc.

	// Cache for selected GleipFlow ID to avoid race conditions
	selectedGleipFlowID string
	selectedGleipMutex  sync.RWMutex

	// Project persistence handler
	persistence *ProjectPersistence
}

// NewApp creates a new App application struct
func NewApp() *App {
	InitSettings()
	app := &App{
		gleipFlowsCache:  make(map[string]*GleipFlow),
		scanTargetsCache: make(map[string]*ScanTarget),
		scanConfigsCache: make(map[string]*ScanConfig),
		scanResultsCache: make(map[string]*ScanResult),
		activeScans:      make(map[string]*ScanResult),
		autoSaveInterval: 5 * time.Second, // Auto-save every 5 seconds at most
	}
	app.gleipFlowExecutor = NewGleipFlowExecutor(app)
	app.persistence = NewProjectPersistence(app)
	return app
}

// Frontend-callable methods for file operations

// NewProject creates a new empty project (callable from frontend)
func (a *App) NewProject() error {
	return a.persistence.CreateNewProject()
}

// SaveProject saves the current project (callable from frontend)
func (a *App) SaveProject() error {
	return a.persistence.SaveProject()
}

// SaveProjectAs saves the current project with a new name/location (callable from frontend)
func (a *App) SaveProjectAs() error {
	return a.persistence.SaveProjectAs()
}

// LoadProject loads a project from disk (callable from frontend)
func (a *App) LoadProject() error {
	return a.persistence.LoadProject()
}

// clearApplicationState clears all the current application state
func (a *App) clearApplicationState() {
	// Clear proxy history by resetting the counter
	if a.proxyServer != nil {
		a.proxyServer.ResetRequestCounter()
	}

	// Clear gleipFlows
	a.gleipFlowsMutex.Lock()
	a.gleipFlowsCache = make(map[string]*GleipFlow)
	a.gleipFlowsMutex.Unlock()

	// Clear scan targets
	a.scanTargetsMutex.Lock()
	a.scanTargetsCache = make(map[string]*ScanTarget)
	a.scanTargetsMutex.Unlock()

	// Clear scan configs
	a.scanConfigsMutex.Lock()
	a.scanConfigsCache = make(map[string]*ScanConfig)
	a.scanConfigsMutex.Unlock()

	// Clear scan results
	a.scanResultsMutex.Lock()
	a.scanResultsCache = make(map[string]*ScanResult)
	a.scanResultsMutex.Unlock()

	// Clear active scans
	a.activeScansMutex.Lock()
	a.activeScans = make(map[string]*ScanResult)
	a.activeScansMutex.Unlock()

	// Reset other application state as needed
}

// requestAutoSave schedules an auto-save operation with debouncing to avoid excessive disk I/O
func (a *App) requestAutoSave() {
	a.autoSaveMutex.Lock()
	defer a.autoSaveMutex.Unlock()

	// If auto-save is in progress, just mark that another save is requested
	if a.autoSaveInProgress {
		a.autoSaveRequested = true
		return
	}

	// If timer is already running, stop it
	if a.autoSaveTimer != nil {
		a.autoSaveTimer.Stop()
	}

	// Start a new timer
	a.autoSaveTimer = time.AfterFunc(a.autoSaveInterval, func() {
		a.performAutoSave()
	})
}

// performAutoSave executes the actual auto-save operation
func (a *App) performAutoSave() {
	a.autoSaveMutex.Lock()
	a.autoSaveInProgress = true
	a.autoSaveRequested = false
	a.autoSaveMutex.Unlock()

	// Check if there are any dirty components before saving
	if !a.hasDirtyComponents() {
		fmt.Printf("Debug: No dirty components, skipping auto-save\n")
		a.autoSaveMutex.Lock()
		a.autoSaveInProgress = false
		a.autoSaveMutex.Unlock()
		return
	}

	// Get dirty components for logging
	dirtyComponents := a.getDirtyComponents()
	fmt.Printf("Debug: Auto-saving dirty components: %v\n", dirtyComponents)

	// Perform the incremental save operation
	err := a.persistence.saveToTempProjectIncremental()
	if err != nil {
		fmt.Printf("Warning: Auto-save failed: %v\n", err)
	} else {
		fmt.Printf("Debug: Auto-save completed successfully\n")
	}

	// Check if another save was requested while we were saving
	a.autoSaveMutex.Lock()
	if a.autoSaveRequested {
		// Reset the flag
		a.autoSaveRequested = false

		// Schedule another save
		a.autoSaveTimer = time.AfterFunc(a.autoSaveInterval, func() {
			a.performAutoSave()
		})
	}
	a.autoSaveInProgress = false
	a.autoSaveMutex.Unlock()
}

// restoreApplicationState restores the application state from the current project
func (a *App) restoreApplicationState() error {
	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	if a.currentProject == nil {
		return fmt.Errorf("no current project to restore")
	}

	// Restore proxy history
	if a.proxyServer != nil && a.currentProject.RequestHistory != nil {
		// Get the transaction store from the proxy server
		if store, ok := a.proxyServer.transactionStore.(*network.InMemoryTransactionStore); ok {
			// Clear existing transactions
			store.Reset()

			// Add all transactions from the project
			for _, tx := range a.currentProject.RequestHistory {
				if tx != nil {
					store.Add(*tx)
				}
			}
		}
	}

	// Restore gleipFlows
	a.gleipFlowsMutex.Lock()
	a.gleipFlowsCache = make(map[string]*GleipFlow)
	for _, gleipFlow := range a.currentProject.GleipFlows {
		a.gleipFlowsCache[gleipFlow.ID] = gleipFlow
	}
	a.gleipFlowsMutex.Unlock()

	// Initialize the selected GleipFlow ID cache
	a.selectedGleipMutex.Lock()
	a.selectedGleipFlowID = a.currentProject.SelectedGleipFlowID
	a.selectedGleipMutex.Unlock()

	// Restore scan targets
	a.scanTargetsMutex.Lock()
	a.scanTargetsCache = make(map[string]*ScanTarget)
	for _, target := range a.currentProject.ScanTargets {
		a.scanTargetsCache[target.ID] = target
	}
	a.scanTargetsMutex.Unlock()

	// Restore scan configs
	a.scanConfigsMutex.Lock()
	a.scanConfigsCache = make(map[string]*ScanConfig)
	for _, config := range a.currentProject.ScanConfigs {
		a.scanConfigsCache[config.ID] = config
	}
	a.scanConfigsMutex.Unlock()

	// Restore scan results
	a.scanResultsMutex.Lock()
	a.scanResultsCache = make(map[string]*ScanResult)
	for _, result := range a.currentProject.ScanResults {
		a.scanResultsCache[result.ID] = result
	}
	a.scanResultsMutex.Unlock()

	return nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx

	// Initialize telemetry
	InitTelemetry()

	// Track app startup
	TrackAppLaunch()

	// Initialize certificate manager
	certManager := cert.NewCertificateManager()
	if err := certManager.LoadCA(); err != nil {
		fmt.Printf("Failed to load CA: %v\n", err)
		return
	}

	// Create dependencies for the proxy server
	transactionStore := network.NewInMemoryTransactionStore()
	interceptQueue := NewInterceptQueue()
	eventEmitter := NewDefaultEventEmitter(a)

	// Create proxy server with dependency injection
	a.proxyServer = NewProxyServer(ctx, 9090, certManager, transactionStore, interceptQueue, eventEmitter)

	// Start proxy server
	if err := a.proxyServer.Start(); err != nil {
		fmt.Printf("Failed to start proxy server: %v\n", err)
	}

	// Initialize project on startup (load existing temporary project or create new one)
	if err := a.persistence.InitializeProjectOnStartup(); err != nil {
		fmt.Printf("Failed to initialize project on startup: %v\n", err)
	}

	// Start auto-save timer
	a.requestAutoSave()

	// Check for updates in the background
	go func() {
		// Wait a bit before checking for updates to not slow down startup
		time.Sleep(5 * time.Second)

		updateInfo, err := a.CheckForUpdates()
		if err != nil {
			fmt.Printf("Failed to check for updates: %v\n", err)
			return
		}

		if updateInfo != nil && updateInfo.IsUpdateNeeded {
			// Emit event to frontend
			rt.EventsEmit(ctx, "update:available", updateInfo)
		}
	}()

	// Update window title to reflect the current project
	a.updateWindowTitle()
}

// GetProxyRequests returns all intercepted requests
func (a *App) GetProxyRequests() []network.HTTPTransactionSummary {
	return a.proxyServer.GetRequests()
}

// GetProxyRequestsAfter returns all intercepted requests after the given ID
func (a *App) GetProxyRequestsAfter(lastID string) []network.HTTPTransactionSummary {
	return a.proxyServer.GetRequestsAfter(lastID)
}

// GetInterceptedRequests returns all requests waiting for interception
func (a *App) GetInterceptedRequests() []*network.HTTPTransaction {
	return a.proxyServer.GetInterceptedRequests()
}

// SetInterceptEnabled enables or disables request interception
func (a *App) SetInterceptEnabled(enabled bool) {
	a.proxyServer.SetInterceptEnabled(enabled)

	// Update project
	a.projectMutex.Lock()
	if a.currentProject != nil {
		a.currentProject.ProxyEnabled = enabled
	}
	a.projectMutex.Unlock()

	// Request an auto-save since project state has changed
	a.requestAutoSaveWithComponent("project_meta")

	if enabled {
		TrackFirefoxAction("intercept_enabled", true)
	} else {
		TrackFirefoxAction("intercept_disabled", true)
	}
}

// GetInterceptEnabled returns the current interception status
func (a *App) GetInterceptEnabled() bool {
	return a.proxyServer.GetInterceptEnabled()
}

// ModifyInterceptedRequest modifies an intercepted request and allows it to continue
func (a *App) ModifyInterceptedRequest(id string, method, url string, headers map[string]string, body string) error {
	return a.proxyServer.ModifyInterceptedRequest(id, method, url, headers, body)
}

// ModifyInterceptedResponse modifies an intercepted response and allows it to continue
func (a *App) ModifyInterceptedResponse(id string, dump string) error {
	err := a.proxyServer.ModifyInterceptedResponse(id, dump)
	if err == nil {
		// Request auto-save since we've modified data
		a.requestAutoSaveWithComponent("request_history")
	}
	return err
}

// ForwardRequestAndWaitForResponse forwards a request and waits for the response to be intercepted
func (a *App) ForwardRequestAndWaitForResponse(id string, method, url string, headers map[string]string, body string) error {
	return a.proxyServer.ForwardInterceptedRequest(id, method, url, headers, body, true)
}

// ForwardRequestImmediately forwards an intercepted request without intercepting the response
func (a *App) ForwardRequestImmediately(id string, method, url string, headers map[string]string, body string) error {
	return a.proxyServer.ForwardInterceptedRequest(id, method, url, headers, body, false)
}

// ForwardInterceptedRequest forwards an intercepted request with optional response interception
func (a *App) ForwardInterceptedRequest(id string, method, url string, headers map[string]string, body string, interceptResponse bool) error {
	return a.proxyServer.ForwardInterceptedRequest(id, method, url, headers, body, interceptResponse)
}

// ExecuteGleipFlow executes a gleipFlow
func (a *App) ExecuteGleipFlow(gleipFlowID string) ([]ExecutionResult, error) {

	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, err
	}

	results, err := a.gleipFlowExecutor.ExecuteGleipFlow(gleipFlow)
	if err == nil {
		// Request auto-save since we've modified data
		a.requestAutoSaveWithComponent("gleip_flows")
	}

	return results, err
}

// SaveScanTarget saves a scan target
func (a *App) SaveScanTarget(target ScanTarget) error {
	// Generate ID if not present
	if target.ID == "" {
		target.ID = uuid.New().String()
	}

	a.scanTargetsMutex.Lock()
	defer a.scanTargetsMutex.Unlock()

	// Update the cache
	a.scanTargetsCache[target.ID] = &target

	// Request auto-save
	a.requestAutoSaveWithComponent("scan_targets")

	// Save to disk
	targetPath := filepath.Join(paths.GlobalPaths.ScanTargetsDir, target.ID+".json")
	data, err := json.MarshalIndent(target, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scan target: %v", err)
	}

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save scan target: %v", err)
	}

	return nil
}

// GetScanTargets returns all scan targets
func (a *App) GetScanTargets() []ScanTarget {
	a.scanTargetsMutex.RLock()
	defer a.scanTargetsMutex.RUnlock()

	targets := make([]ScanTarget, 0, len(a.scanTargetsCache))
	for _, target := range a.scanTargetsCache {
		targets = append(targets, *target)
	}

	return targets
}

// GetScanTarget returns a specific scan target
func (a *App) GetScanTarget(id string) (*ScanTarget, error) {
	a.scanTargetsMutex.RLock()
	defer a.scanTargetsMutex.RUnlock()

	target, exists := a.scanTargetsCache[id]
	if !exists {
		return nil, fmt.Errorf("scan target not found: %s", id)
	}

	return target, nil
}

// DeleteScanTarget deletes a scan target
func (a *App) DeleteScanTarget(id string) error {
	a.scanTargetsMutex.Lock()
	defer a.scanTargetsMutex.Unlock()

	// Check if target exists
	_, exists := a.scanTargetsCache[id]
	if !exists {
		return fmt.Errorf("scan target not found: %s", id)
	}

	// Delete from cache
	delete(a.scanTargetsCache, id)

	// Request auto-save
	a.requestAutoSaveWithComponent("scan_targets")

	// Delete from disk
	targetPath := filepath.Join(paths.GlobalPaths.ScanTargetsDir, id+".json")
	if err := os.Remove(targetPath); err != nil {
		return fmt.Errorf("failed to delete scan target file: %v", err)
	}

	return nil
}

// SaveScanConfig saves a scan configuration
func (a *App) SaveScanConfig(config ScanConfig) error {
	// Generate ID if not present
	if config.ID == "" {
		config.ID = uuid.New().String()
	}

	a.scanConfigsMutex.Lock()
	defer a.scanConfigsMutex.Unlock()

	// Update the cache
	a.scanConfigsCache[config.ID] = &config

	// Request auto-save
	a.requestAutoSaveWithComponent("scan_configs")

	// Save to disk
	configPath := filepath.Join(paths.GlobalPaths.ScanConfigsDir, config.ID+".json")
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scan config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save scan config: %v", err)
	}

	return nil
}

// GetScanConfigs returns all scan configurations
func (a *App) GetScanConfigs() []ScanConfig {
	a.scanConfigsMutex.RLock()
	defer a.scanConfigsMutex.RUnlock()

	configs := make([]ScanConfig, 0, len(a.scanConfigsCache))
	for _, config := range a.scanConfigsCache {
		configs = append(configs, *config)
	}

	return configs
}

// GetScanConfig returns a specific scan configuration
func (a *App) GetScanConfig(id string) (*ScanConfig, error) {
	a.scanConfigsMutex.RLock()
	defer a.scanConfigsMutex.RUnlock()

	config, exists := a.scanConfigsCache[id]
	if !exists {
		return nil, fmt.Errorf("scan config not found: %s", id)
	}

	return config, nil
}

// DeleteScanConfig deletes a scan configuration
func (a *App) DeleteScanConfig(id string) error {
	a.scanConfigsMutex.Lock()
	defer a.scanConfigsMutex.Unlock()

	// Check if config exists
	_, exists := a.scanConfigsCache[id]
	if !exists {
		return fmt.Errorf("scan config not found: %s", id)
	}

	// Delete from cache
	delete(a.scanConfigsCache, id)

	// Request auto-save
	a.requestAutoSaveWithComponent("scan_configs")

	// Delete from disk
	configPath := filepath.Join(paths.GlobalPaths.ScanConfigsDir, id+".json")
	if err := os.Remove(configPath); err != nil {
		return fmt.Errorf("failed to delete scan config file: %v", err)
	}

	return nil
}

// SaveScanResult saves a scan result
func (a *App) SaveScanResult(result ScanResult) error {
	a.scanResultsMutex.Lock()
	defer a.scanResultsMutex.Unlock()

	// Update the cache
	a.scanResultsCache[result.ID] = &result

	// Request auto-save
	a.requestAutoSaveWithComponent("scan_results")

	// Save to disk
	resultPath := filepath.Join(paths.GlobalPaths.ScanResultsDir, result.ID+".json")
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scan result: %v", err)
	}

	if err := os.WriteFile(resultPath, data, 0644); err != nil {
		return fmt.Errorf("failed to save scan result: %v", err)
	}

	return nil
}

// StartScan starts an automatic scan
func (a *App) StartScan(configID string) (*ScanResult, error) {
	// Get scan configuration
	config, err := a.GetScanConfig(configID)
	if err != nil {
		return nil, err
	}

	// Get target
	target, err := a.GetScanTarget(config.TargetID)
	if err != nil {
		return nil, err
	}

	// Create scan result
	result := &ScanResult{
		ID:           uuid.New().String(),
		ScanConfigID: configID,
		StartTime:    time.Now().Format(time.RFC3339),
		Status:       "running",
	}

	// Add to active scans
	a.activeScansMutex.Lock()
	a.activeScans[result.ID] = result
	a.activeScansMutex.Unlock()

	// Start scan in a goroutine
	go func() {
		a.runScan(result, *config, *target)

		// Remove from active scans when done
		a.activeScansMutex.Lock()
		delete(a.activeScans, result.ID)
		a.activeScansMutex.Unlock()

		// Save the final result
		if err := a.SaveScanResult(*result); err != nil {
			fmt.Printf("Failed to save scan result: %v\n", err)
		}
	}()

	return result, nil
}

// GetScanResult gets the result of a scan
func (a *App) GetScanResult(id string) (*ScanResult, error) {
	// First check active scans
	a.activeScansMutex.RLock()
	result, exists := a.activeScans[id]
	a.activeScansMutex.RUnlock()

	if exists {
		return result, nil
	}

	// Then check completed scans
	a.scanResultsMutex.RLock()
	defer a.scanResultsMutex.RUnlock()

	result, exists = a.scanResultsCache[id]
	if !exists {
		return nil, fmt.Errorf("scan result not found: %s", id)
	}

	return result, nil
}

// GetScanResults gets all scan results
func (a *App) GetScanResults() []ScanResult {
	// Get completed scans
	a.scanResultsMutex.RLock()
	results := make([]ScanResult, 0, len(a.scanResultsCache))
	for _, result := range a.scanResultsCache {
		results = append(results, *result)
	}
	a.scanResultsMutex.RUnlock()

	// Add active scans
	a.activeScansMutex.RLock()
	for _, result := range a.activeScans {
		results = append(results, *result)
	}
	a.activeScansMutex.RUnlock()

	return results
}

// runScan runs an automatic scan
func (a *App) runScan(result *ScanResult, config ScanConfig, target ScanTarget) {
	// Set up crawling and scanning
	visitedURLs := make(map[string]bool)
	urlQueue := []string{target.BaseURL}

	// Authenticate if needed
	var authCookies []*http.Cookie
	if config.Authentication != nil {
		// TODO: Implement authentication and store cookies/tokens
	}

	// Process URLs up to max depth
	depth := 0
	for depth <= config.MaxDepth && len(urlQueue) > 0 && result.RequestCount < config.MaxRequests {
		// Get next batch of URLs at current depth
		urlsAtDepth := urlQueue
		urlQueue = []string{}

		for _, url := range urlsAtDepth {
			// Skip if already visited
			if visitedURLs[url] {
				continue
			}

			// Mark as visited
			visitedURLs[url] = true

			// Test for vulnerabilities
			newVulns, newURLs, err := a.testURL(url, config, target, authCookies)
			if err != nil {
				result.ErrorCount++
				continue
			}

			// Add vulnerabilities to result
			result.Vulnerabilities = append(result.Vulnerabilities, newVulns...)

			// Add new URLs to queue for next depth
			for _, newURL := range newURLs {
				if !visitedURLs[newURL] {
					urlQueue = append(urlQueue, newURL)
				}
			}

			// Increment request count
			result.RequestCount++

			// Respect delay between requests
			if config.RequestDelay > 0 {
				time.Sleep(time.Duration(config.RequestDelay) * time.Millisecond)
			}

			// Check if we've reached the request limit
			if result.RequestCount >= config.MaxRequests {
				break
			}
		}

		depth++
	}

	// Mark scan as completed
	result.Status = "completed"
	result.EndTime = time.Now().Format(time.RFC3339)

	// TODO: Save result to storage
}

// testURL tests a URL for vulnerabilities
func (a *App) testURL(url string, config ScanConfig, target ScanTarget, authCookies []*http.Cookie) ([]Vulnerability, []string, error) {
	vulnerabilities := []Vulnerability{}
	discoveredURLs := []string{}

	// Create HTTP client
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects automatically
		},
	}

	// First make a regular GET request to the URL
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, nil, err
	}

	// Add cookies for authentication
	for _, cookie := range authCookies {
		req.AddCookie(cookie)
	}

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	// Extract links from response
	// TODO: Implement HTML parsing to extract links

	// Test for vulnerabilities based on config
	for _, vulnType := range config.VulnerabilityTypes {
		switch vulnType {
		case XSS:
			xssVulns := a.testXSS(url, client, authCookies)
			vulnerabilities = append(vulnerabilities, xssVulns...)
		case SQLInjection:
			sqlVulns := a.testSQLInjection(url, client, authCookies)
			vulnerabilities = append(vulnerabilities, sqlVulns...)
		case CSRF:
			csrfVulns := a.testCSRF(url, client, authCookies)
			vulnerabilities = append(vulnerabilities, csrfVulns...)
		case PathTraversal:
			ptVulns := a.testPathTraversal(url, client, authCookies)
			vulnerabilities = append(vulnerabilities, ptVulns...)
		case OpenRedirect:
			orVulns := a.testOpenRedirect(url, client, authCookies)
			vulnerabilities = append(vulnerabilities, orVulns...)
		}
	}

	return vulnerabilities, discoveredURLs, nil
}

// testXSS tests for XSS vulnerabilities
func (a *App) testXSS(url string, client *http.Client, authCookies []*http.Cookie) []Vulnerability {
	// TODO: Implement XSS testing
	return []Vulnerability{}
}

// testSQLInjection tests for SQL injection vulnerabilities
func (a *App) testSQLInjection(url string, client *http.Client, authCookies []*http.Cookie) []Vulnerability {
	// TODO: Implement SQL injection testing
	return []Vulnerability{}
}

// testCSRF tests for CSRF vulnerabilities
func (a *App) testCSRF(url string, client *http.Client, authCookies []*http.Cookie) []Vulnerability {
	// TODO: Implement CSRF testing
	return []Vulnerability{}
}

// testPathTraversal tests for path traversal vulnerabilities
func (a *App) testPathTraversal(url string, client *http.Client, authCookies []*http.Cookie) []Vulnerability {
	// TODO: Implement path traversal testing
	return []Vulnerability{}
}

// testOpenRedirect tests for open redirect vulnerabilities
func (a *App) testOpenRedirect(url string, client *http.Client, authCookies []*http.Cookie) []Vulnerability {
	// TODO: Implement open redirect testing
	return []Vulnerability{}
}

// GetTransactionDetails returns the full details for a specific transaction ID
func (a *App) GetTransactionDetails(id string) (*network.HTTPTransaction, error) {
	return a.proxyServer.GetTransactionDetails(id)
}

// GetTransactionMetadata returns metadata about a transaction's data sizes for chunked loading
func (a *App) GetTransactionMetadata(id string) (map[string]interface{}, error) {
	return a.proxyServer.GetTransactionMetadata(id)
}

// GetTransactionChunk returns a chunk of request or response data
func (a *App) GetTransactionChunk(id string, dataType string, chunkIndex int) (*network.HTTPTransactionChunk, error) {
	return a.proxyServer.GetTransactionChunk(id, dataType, chunkIndex)
}

// SearchProxyRequests searches through proxy request history
func (a *App) SearchProxyRequests(query string) []network.HTTPTransactionSummary {
	if a.proxyServer == nil {
		return []network.HTTPTransactionSummary{}
	}

	// Get all transactions from the store
	allTransactions := a.proxyServer.transactionStore.GetAll()

	if query == "" {
		// Return all as summaries
		summaries := make([]network.HTTPTransactionSummary, len(allTransactions))
		for i, tx := range allTransactions {
			summary := network.HTTPTransactionSummary{
				ID:        tx.ID,
				Timestamp: tx.Timestamp,
				Method:    tx.Request.Method(),
				URL:       tx.Request.URL(),
				SeqNumber: tx.SeqNumber,
			}
			if tx.Response != nil {
				statusCode := tx.Response.StatusCode()
				summary.StatusCode = &statusCode
				status := tx.Response.Status()
				summary.Status = &status
				summary.ResponseSize = len(tx.Response.Dump)
			}
			summaries[i] = summary
		}
		return summaries
	}

	// Search through transactions
	var results []network.HTTPTransactionSummary
	queryLower := strings.ToLower(query)

	for _, tx := range allTransactions {
		// Search in various fields
		if strings.Contains(strings.ToLower(tx.Request.Method()), queryLower) ||
			strings.Contains(strings.ToLower(tx.Request.URL()), queryLower) ||
			strings.Contains(strings.ToLower(tx.Request.Dump), queryLower) ||
			(tx.Response != nil && strings.Contains(strings.ToLower(tx.Response.Dump), queryLower)) {

			summary := network.HTTPTransactionSummary{
				ID:        tx.ID,
				Timestamp: tx.Timestamp,
				Method:    tx.Request.Method(),
				URL:       tx.Request.URL(),
				SeqNumber: tx.SeqNumber,
			}
			if tx.Response != nil {
				statusCode := tx.Response.StatusCode()
				summary.StatusCode = &statusCode
				status := tx.Response.Status()
				summary.Status = &status
				summary.ResponseSize = len(tx.Response.Dump)
			}
			results = append(results, summary)
		}
	}

	return results
}

// GetCurrentTelemetryConfig returns the current telemetry configuration
func (a *App) GetCurrentTelemetryConfig() map[string]interface{} {
	sc := NewSettingsController()
	settings := sc.GetSettings()

	return map[string]interface{}{
		"enabled": settings.TelemetryEnabled,
	}
}

// ManualCheckForUpdates allows the user to manually check for updates
func (a *App) ManualCheckForUpdates() (*UpdateInfo, error) {
	updateInfo, err := a.CheckForUpdates()
	if err != nil {
		TrackError("auto_update", "manual_check_failed")
		return nil, err
	}

	// If there's an update available, emit the event to show the update modal
	if updateInfo.IsUpdateNeeded {
		rt.EventsEmit(a.ctx, "update:available", updateInfo)
	}

	// Always return the UpdateInfo so the frontend can show an appropriate message
	return updateInfo, nil
}

// InstallUpdate starts the update process
func (a *App) InstallUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("download URL is empty")
	}

	return a.DownloadAndInstallUpdate(a.ctx, downloadURL)
}

// shutdown is called when the app is about to close
func (a *App) Shutdown(ctx context.Context) {
	// Instead of prompting during shutdown (which can cause crashes),
	// we'll just make sure the temp file is saved.
	// The user will be prompted about temp project recovery on next startup.
	a.projectMutex.RLock()
	projectExists := a.currentProject != nil
	a.projectMutex.RUnlock()

	if projectExists {
		// Final auto-save to ensure we don't lose state
		err := a.persistence.saveToTempProject()
		if err != nil {
			fmt.Printf("Warning: Failed to save project during shutdown: %v\n", err)
		} else {
			// Check if we're saving to user's file or temp file
			defaultTempPath := filepath.Join(paths.GlobalPaths.AppDataDir, "temp_project.gleip")
			if a.tempProjectPath == defaultTempPath {
				fmt.Printf("Successfully saved unsaved project to temp file during shutdown\n")
			} else {
				fmt.Printf("Successfully saved project to %s during shutdown\n", a.tempProjectPath)
			}
		}
	}

	// Kill Firefox if it's running
	if a.IsFirefoxRunning() {
		// Get the process
		process, err := os.FindProcess(a.firefoxPID)
		if err == nil {
			// Kill the process
			if err := process.Kill(); err != nil {
				fmt.Printf("Warning: Failed to terminate Firefox process: %v\n", err)
			} else {
				fmt.Printf("Successfully terminated Firefox process with PID: %d\n", a.firefoxPID)
				// Reset the PID
				a.firefoxPID = 0
			}
		}
	}

	// Ensure telemetry is properly shutdown
	ShutdownTelemetry()
}

// BrowseForAPICollectionFile presents a file dialog to select an API collection file (OpenAPI or Postman)
func (a *App) BrowseForAPICollectionFile() (string, error) {
	filePath, err := rt.OpenFileDialog(a.ctx, rt.OpenDialogOptions{
		Title: "Import API Collection",
		Filters: []rt.FileFilter{
			{
				DisplayName: "API Collection Files (*.json, *.yaml, *.yml)",
				Pattern:     "*.json;*.yaml;*.yml",
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}

	if filePath == "" {
		return "", fmt.Errorf("no file selected")
	}

	return filePath, nil
}

// BrowseForOpenAPIFile presents a file dialog to select an OpenAPI specification file
// Deprecated: Use BrowseForAPICollectionFile instead
func (a *App) BrowseForOpenAPIFile() (string, error) {
	return a.BrowseForAPICollectionFile()
}

// SetSelectedAPICollection sets the active API collection in the project
func (a *App) SetSelectedAPICollection(collectionID string) error {
	a.projectMutex.Lock()
	defer a.projectMutex.Unlock()

	if a.currentProject == nil {
		return fmt.Errorf("cannot set selected collection: no active project. This may indicate an application initialization issue")
	}

	// Update the selected collection ID
	a.currentProject.SelectedAPICollection = collectionID

	// Request auto-save
	a.requestAutoSaveWithComponent("project_meta")

	// Save the project
	if err := a.persistence.saveToTempProject(); err != nil {
		return fmt.Errorf("failed to save to temp project: %w", err)
	}

	return nil
}

// GetSelectedAPICollection gets the active API collection ID from the project
func (a *App) GetSelectedAPICollection() string {
	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	if a.currentProject == nil {
		return ""
	}

	return a.currentProject.SelectedAPICollection
}

// Add a function to the App struct that acts as a callback for new transactions
func (a *App) handleNewTransaction(transaction network.HTTPTransaction) {
	// Request auto-save with request history marked as dirty
	a.requestAutoSaveWithComponent("request_history")
}

// SetSelectedGleipFlowID sets the active GleipFlow ID in the project
func (a *App) SetSelectedGleipFlowID(gleipFlowID string) error {
	// Update the cache first
	a.selectedGleipMutex.Lock()
	a.selectedGleipFlowID = gleipFlowID
	a.selectedGleipMutex.Unlock()

	// Update the project
	a.projectMutex.Lock()
	if a.currentProject != nil {
		a.currentProject.SelectedGleipFlowID = gleipFlowID
	}
	a.projectMutex.Unlock()

	// Request auto-save (non-blocking)
	a.requestAutoSaveWithComponent("project_meta")

	return nil
}

// GetSelectedGleipFlowID gets the active GleipFlow ID from the project
func (a *App) GetSelectedGleipFlowID() string {
	// Try cache first (fastest)
	a.selectedGleipMutex.RLock()
	cachedID := a.selectedGleipFlowID
	a.selectedGleipMutex.RUnlock()

	if cachedID != "" {
		return cachedID
	}

	// Fallback to project data
	a.projectMutex.RLock()
	projectID := ""
	if a.currentProject != nil {
		projectID = a.currentProject.SelectedGleipFlowID
	}
	a.projectMutex.RUnlock()

	// Update cache with project value if we found one
	if projectID != "" {
		a.selectedGleipMutex.Lock()
		a.selectedGleipFlowID = projectID
		a.selectedGleipMutex.Unlock()
	}

	return projectID
}

// Dirty state management methods

// markDirty marks specific components as dirty for incremental saving
func (a *App) markDirty(component string) {
	a.dirtyStateMutex.Lock()
	defer a.dirtyStateMutex.Unlock()

	switch component {
	case "request_history":
		a.dirtyRequestHistory = true
	case "gleip_flows":
		a.dirtyGleipFlows = true
	case "api_collections":
		a.dirtyAPICollections = true
	case "scan_targets":
		a.dirtyScanTargets = true
	case "scan_configs":
		a.dirtyScanConfigs = true
	case "scan_results":
		a.dirtyScanResults = true
	case "project_meta":
		a.dirtyProjectMeta = true
	}
}

// clearDirtyFlags clears all dirty flags after a successful save
func (a *App) clearDirtyFlags() {
	a.dirtyStateMutex.Lock()
	defer a.dirtyStateMutex.Unlock()

	a.dirtyRequestHistory = false
	a.dirtyGleipFlows = false
	a.dirtyAPICollections = false
	a.dirtyScanTargets = false
	a.dirtyScanConfigs = false
	a.dirtyScanResults = false
	a.dirtyProjectMeta = false
}

// getDirtyComponents returns a list of components that need saving
func (a *App) getDirtyComponents() []string {
	a.dirtyStateMutex.RLock()
	defer a.dirtyStateMutex.RUnlock()

	var dirty []string
	if a.dirtyRequestHistory {
		dirty = append(dirty, "request_history")
	}
	if a.dirtyGleipFlows {
		dirty = append(dirty, "gleip_flows")
	}
	if a.dirtyAPICollections {
		dirty = append(dirty, "api_collections")
	}
	if a.dirtyScanTargets {
		dirty = append(dirty, "scan_targets")
	}
	if a.dirtyScanConfigs {
		dirty = append(dirty, "scan_configs")
	}
	if a.dirtyScanResults {
		dirty = append(dirty, "scan_results")
	}
	if a.dirtyProjectMeta {
		dirty = append(dirty, "project_meta")
	}
	return dirty
}

// hasDirtyComponents checks if any components are dirty
func (a *App) hasDirtyComponents() bool {
	a.dirtyStateMutex.RLock()
	defer a.dirtyStateMutex.RUnlock()

	return a.dirtyRequestHistory || a.dirtyGleipFlows || a.dirtyAPICollections ||
		a.dirtyScanTargets || a.dirtyScanConfigs || a.dirtyScanResults || a.dirtyProjectMeta
}

// requestAutoSaveWithComponent schedules an auto-save and marks a component as dirty
func (a *App) requestAutoSaveWithComponent(component string) {
	a.markDirty(component)
	a.requestAutoSave()
}

// CreateHTTPRequest creates a proper HTTPRequest object with methods attached
func (a *App) CreateHTTPRequest(host string, tls bool, dump string) *network.HTTPRequest {
	return &network.HTTPRequest{
		Host: host,
		TLS:  tls,
		Dump: dump,
	}
}

// updateWindowTitle updates the window title to show "Gleip - [project filename]"
func (a *App) updateWindowTitle() {
	if a.ctx == nil {
		return
	}

	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	var title string
	if a.currentProject != nil && a.currentProject.Name != "" {
		title = fmt.Sprintf("Gleip - %s", a.currentProject.Name)
	} else {
		title = "Gleip"
	}

	rt.WindowSetTitle(a.ctx, title)
}

// GetAppVersion returns the current application version information
func (a *App) GetAppVersion() map[string]interface{} {
	return map[string]interface{}{
		"version":     AppVersion,
		"fullVersion": AppVersion,
	}
}
