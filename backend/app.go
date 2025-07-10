package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/gleipflow"
	"Gleip/backend/network"
	"Gleip/backend/network/http_utils"
	"Gleip/backend/paths"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"Gleip/backend/cert"

	"strconv"

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

	// Request History Sorting state
	SortColumn             string `json:"sortColumn,omitempty"`             // Current primary sort column
	SortDirection          string `json:"sortDirection,omitempty"`          // "asc", "desc", or ""
	SecondarySortColumn    string `json:"secondarySortColumn,omitempty"`    // Always the last chosen sort for "#" column
	SecondarySortDirection string `json:"secondarySortDirection,omitempty"` // Direction for secondary sort

	// Request GleipFlow Tab data
	GleipFlows          []*GleipFlow      `json:"gleipFlows,omitempty"`
	Variables           map[string]string `json:"variables,omitempty"`
	SelectedGleipFlowID string            `json:"selectedGleipFlowId,omitempty"`

	// API Collections data
	APICollections        []APICollection `json:"apiCollections,omitempty"`
	SelectedAPICollection string          `json:"selectedApiCollectionId,omitempty"`

	// Other application state
	ProxyEnabled bool `json:"proxyEnabled"`
	ProxyPort    int  `json:"proxyPort"`
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
	// scanTargetsCache    map[string]*ScanTarget
	// scanConfigsCache    map[string]*ScanConfig
	// scanResultsCache    map[string]*ScanResult
	// activeScans         map[string]*ScanResult
	firefoxPID        int
	gleipFlowExecutor *GleipFlowExecutor
	tempProjectPath   string // Path to temporary project file

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
		fmt.Printf("DEBUG: Restored flow %s with %d execution results\n", gleipFlow.ID, len(gleipFlow.ExecutionResults))
	}
	a.gleipFlowsMutex.Unlock()

	// Initialize the selected GleipFlow ID cache
	a.selectedGleipMutex.Lock()
	a.selectedGleipFlowID = a.currentProject.SelectedGleipFlowID
	a.selectedGleipMutex.Unlock()

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

// GetInterceptedRequests returns all intercepted requests waiting for user action
func (a *App) GetInterceptedRequests() []*network.HTTPTransaction {
	if a.proxyServer == nil {
		return []*network.HTTPTransaction{}
	}

	result := a.proxyServer.GetInterceptedRequests()
	if result == nil {
		return []*network.HTTPTransaction{}
	}

	return result
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

// ProcessInterceptedResponseForDisplay processes a raw response dump for display in intercept tab
func (a *App) ProcessInterceptedResponseForDisplay(rawResponseDump string) string {
	// Always decompress intercepted responses
	return http_utils.GetPrintableResponseWithDecompression([]byte(rawResponseDump), true)
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

// DropRequest drops an intercepted request entirely without forwarding it
func (a *App) DropRequest(id string) error {
	return a.proxyServer.DropInterceptedRequest(id)
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
		// DIRECTLY update the cached GleipFlow with execution results AND updated steps (action previews)
		a.gleipFlowsMutex.Lock()
		if cachedFlow, exists := a.gleipFlowsCache[gleipFlowID]; exists {
			// Merge new results with existing results (preserve results for non-executed steps)
			cachedFlow.ExecutionResults = a.mergeExecutionResults(cachedFlow.ExecutionResults, results)
			cachedFlow.Steps = gleipFlow.Steps // Update steps array (contains updated action previews)
			// Merge execution result variables into flow variables
			if cachedFlow.MergeExecutionResultsIntoFlowVariables() {
				fmt.Printf("Merged execution result variables into cached flow %s\n", gleipFlowID)
			}
		}
		a.gleipFlowsMutex.Unlock()

		// DIRECTLY update the project GleipFlow with execution results AND updated steps (action previews)
		a.projectMutex.Lock()
		if a.currentProject != nil {
			for _, projectFlow := range a.currentProject.GleipFlows {
				if projectFlow.ID == gleipFlowID {
					// Merge new results with existing results (preserve results for non-executed steps)
					projectFlow.ExecutionResults = a.mergeExecutionResults(projectFlow.ExecutionResults, results)
					projectFlow.Steps = gleipFlow.Steps // Update steps array (contains updated action previews)
					// Merge execution result variables into flow variables
					if projectFlow.MergeExecutionResultsIntoFlowVariables() {
						fmt.Printf("Merged execution result variables into project flow %s\n", gleipFlowID)
					}
					break
				}
			}
		}
		a.projectMutex.Unlock()

		// Request auto-save since we've modified gleipFlow data (including execution results and action previews)
		a.requestAutoSaveWithComponent("gleip_flows")
	}

	return results, err
}

// ExecuteSingleStep executes a single step in a gleipFlow
// It will execute only the specified step, plus any selected non-request steps for context
func (a *App) ExecuteSingleStep(gleipFlowID string, stepIndex int) ([]ExecutionResult, error) {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, err
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return nil, fmt.Errorf("invalid step index: %d", stepIndex)
	}

	targetStep := gleipFlow.Steps[stepIndex]
	if targetStep.StepType != "request" {
		return nil, fmt.Errorf("can only execute request steps with ExecuteSingleStep, got: %s", targetStep.StepType)
	}

	// Create a copy of the flow with modified selections
	flowCopy := *gleipFlow
	flowCopy.Steps = make([]GleipFlowStep, len(gleipFlow.Steps))
	copy(flowCopy.Steps, gleipFlow.Steps)

	// Modify selections: only target request step + selected non-request steps
	for i := range flowCopy.Steps {
		if i == stepIndex {
			// Always select the target request step
			flowCopy.Steps[i].Selected = true
		} else if flowCopy.Steps[i].StepType == "request" {
			// Deselect all other request steps
			flowCopy.Steps[i].Selected = false
		}
		// Non-request steps keep their original selection for context
	}

	// Execute with the modified selections
	results, err := a.gleipFlowExecutor.ExecuteGleipFlow(&flowCopy)
	if err == nil {
		// DIRECTLY update the cached GleipFlow with execution results AND updated steps (action previews)
		// but preserve the original Selected properties
		a.gleipFlowsMutex.Lock()
		if cachedFlow, exists := a.gleipFlowsCache[gleipFlowID]; exists {
			// Merge new results with existing results (preserve results for non-executed steps)
			cachedFlow.ExecutionResults = a.mergeExecutionResults(cachedFlow.ExecutionResults, results)
			// Update steps array but preserve original Selected properties
			for i := range cachedFlow.Steps {
				if i < len(flowCopy.Steps) {
					// Preserve original Selected property
					originalSelected := cachedFlow.Steps[i].Selected
					cachedFlow.Steps[i] = flowCopy.Steps[i]
					cachedFlow.Steps[i].Selected = originalSelected
				}
			}
			// Merge execution result variables into flow variables
			if cachedFlow.MergeExecutionResultsIntoFlowVariables() {
				fmt.Printf("Merged execution result variables into cached flow %s\n", gleipFlowID)
			}
		}
		a.gleipFlowsMutex.Unlock()

		// DIRECTLY update the project GleipFlow with execution results AND updated steps (action previews)
		// but preserve the original Selected properties
		a.projectMutex.Lock()
		if a.currentProject != nil {
			for _, projectFlow := range a.currentProject.GleipFlows {
				if projectFlow.ID == gleipFlowID {
					// Merge new results with existing results (preserve results for non-executed steps)
					projectFlow.ExecutionResults = a.mergeExecutionResults(projectFlow.ExecutionResults, results)
					// Update steps array but preserve original Selected properties
					for i := range projectFlow.Steps {
						if i < len(flowCopy.Steps) {
							// Preserve original Selected property
							originalSelected := projectFlow.Steps[i].Selected
							projectFlow.Steps[i] = flowCopy.Steps[i]
							projectFlow.Steps[i].Selected = originalSelected
						}
					}
					// Merge execution result variables into flow variables
					if projectFlow.MergeExecutionResultsIntoFlowVariables() {
						fmt.Printf("Merged execution result variables into project flow %s\n", gleipFlowID)
					}
					break
				}
			}
		}
		a.projectMutex.Unlock()

		// Request auto-save since we've modified gleipFlow data (including execution results and action previews)
		a.requestAutoSaveWithComponent("gleip_flows")
	}

	return results, err
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
	filters := RequestFilters{Query: query}
	return a.SearchProxyRequestsWithSort(filters, "", "")
}

// SearchProxyRequestsWithSort searches through proxy request history with sorting and filtering
func (a *App) SearchProxyRequestsWithSort(filters RequestFilters, sortColumn string, sortDirection string) []network.HTTPTransactionSummary {
	if a.proxyServer == nil {
		return []network.HTTPTransactionSummary{}
	}

	// Get all transactions from the store
	allTransactions := a.proxyServer.transactionStore.GetAll()

	// Apply search filter (now unified with filters)
	var filteredTransactions []network.HTTPTransaction
	if filters.Query == "" {
		filteredTransactions = allTransactions
	} else {
		queryLower := strings.ToLower(filters.Query)
		for _, tx := range allTransactions {
			// Search in various fields
			if strings.Contains(strings.ToLower(tx.Request.Method()), queryLower) ||
				strings.Contains(strings.ToLower(tx.Request.URL()), queryLower) ||
				strings.Contains(strings.ToLower(tx.Request.Dump), queryLower) ||
				(tx.Response != nil && strings.Contains(strings.ToLower(tx.Response.Dump), queryLower)) {
				filteredTransactions = append(filteredTransactions, tx)
			}
		}
	}

	// Apply advanced filters
	filteredTransactions = a.applyRequestFilters(filteredTransactions, filters)

	// Apply sorting
	a.sortTransactions(filteredTransactions, sortColumn, sortDirection)

	// Convert to summaries
	summaries := make([]network.HTTPTransactionSummary, len(filteredTransactions))
	for i, tx := range filteredTransactions {
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

// SetRequestHistorySorting sets the sorting state for request history
func (a *App) SetRequestHistorySorting(column string, direction string) error {
	a.projectMutex.Lock()
	defer a.projectMutex.Unlock()

	if a.currentProject == nil {
		return fmt.Errorf("no active project")
	}

	// If clicking on the same column, toggle between asc and desc
	if a.currentProject.SortColumn == column {
		switch a.currentProject.SortDirection {
		case "asc":
			direction = "desc"
		case "desc":
			direction = "asc"
		default:
			direction = "asc" // Default to asc if somehow empty
		}
	} else {
		// New column, start with asc
		direction = "asc"
	}

	// If this is the "#" column, also update the secondary sort
	if column == "id" {
		a.currentProject.SecondarySortColumn = column
		a.currentProject.SecondarySortDirection = direction
	}

	// If this is NOT the "#" column, keep the secondary sort as the last chosen sort for "#" column
	// Secondary sort should always be the "#" column with its last chosen direction

	// Update primary sort
	a.currentProject.SortColumn = column
	a.currentProject.SortDirection = direction

	// Request auto-save since sorting state has changed
	a.requestAutoSaveWithComponent("project_meta")

	return nil
}

// GetRequestHistorySorting gets the current sorting state for request history
func (a *App) GetRequestHistorySorting() map[string]string {
	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	if a.currentProject == nil {
		return map[string]string{
			"sortColumn":             "id",
			"sortDirection":          "desc",
			"secondarySortColumn":    "id",
			"secondarySortDirection": "desc",
		}
	}

	// Ensure we never return empty direction strings
	sortDirection := a.currentProject.SortDirection
	if sortDirection == "" {
		sortDirection = "desc"
	}

	secondarySortDirection := a.currentProject.SecondarySortDirection
	if secondarySortDirection == "" {
		secondarySortDirection = "desc"
	}

	return map[string]string{
		"sortColumn":             a.currentProject.SortColumn,
		"sortDirection":          sortDirection,
		"secondarySortColumn":    a.currentProject.SecondarySortColumn,
		"secondarySortDirection": secondarySortDirection,
	}
}

// sortTransactions sorts the transactions based on the specified column and direction
func (a *App) sortTransactions(transactions []network.HTTPTransaction, primaryColumn string, primaryDirection string) {
	// Get current sorting state from project
	a.projectMutex.RLock()
	var secondaryColumn, secondaryDirection string
	if a.currentProject != nil {
		secondaryColumn = a.currentProject.SecondarySortColumn
		secondaryDirection = a.currentProject.SecondarySortDirection
	}
	a.projectMutex.RUnlock()

	// Set defaults if not specified
	if primaryColumn == "" {
		primaryColumn = "id"
	}
	if primaryDirection == "" {
		primaryDirection = "desc"
	}
	if secondaryColumn == "" {
		secondaryColumn = "id"
	}
	if secondaryDirection == "" {
		secondaryDirection = "desc"
	}

	sort.Slice(transactions, func(i, j int) bool {
		// First, try primary sort
		if primaryColumn != "" && primaryDirection != "" {
			result := a.compareTransactions(transactions[i], transactions[j], primaryColumn, primaryDirection)
			if result != 0 {
				return result < 0
			}
		}

		// If primary sort is equal or not specified, use secondary sort
		if secondaryColumn != "" && secondaryDirection != "" && secondaryColumn != primaryColumn {
			result := a.compareTransactions(transactions[i], transactions[j], secondaryColumn, secondaryDirection)
			return result < 0
		}

		// Default to ID descending if all else fails
		return transactions[i].SeqNumber > transactions[j].SeqNumber
	})
}

// compareTransactions compares two transactions based on the specified column and direction
func (a *App) compareTransactions(tx1, tx2 network.HTTPTransaction, column string, direction string) int {
	var result int

	switch column {
	case "id":
		if tx1.SeqNumber < tx2.SeqNumber {
			result = -1
		} else if tx1.SeqNumber > tx2.SeqNumber {
			result = 1
		} else {
			result = 0
		}
	case "host":
		host1 := a.extractHost(tx1.Request.URL())
		host2 := a.extractHost(tx2.Request.URL())
		result = strings.Compare(host1, host2)
	case "method":
		result = strings.Compare(tx1.Request.Method(), tx2.Request.Method())
	case "url":
		path1 := a.extractPath(tx1.Request.URL())
		path2 := a.extractPath(tx2.Request.URL())
		result = strings.Compare(path1, path2)
	case "params":
		hasParams1 := a.hasParams(tx1)
		hasParams2 := a.hasParams(tx2)
		if hasParams1 && !hasParams2 {
			result = -1
		} else if !hasParams1 && hasParams2 {
			result = 1
		} else {
			result = 0
		}
	case "bytes":
		size1 := 0
		size2 := 0
		if tx1.Response != nil {
			size1 = len(tx1.Response.Dump)
		}
		if tx2.Response != nil {
			size2 = len(tx2.Response.Dump)
		}
		if size1 < size2 {
			result = -1
		} else if size1 > size2 {
			result = 1
		} else {
			result = 0
		}
	case "time":
		// Since timestamps are strings, use string comparison (works for ISO 8601)
		result = strings.Compare(tx1.Timestamp, tx2.Timestamp)
	default:
		result = 0
	}

	// Apply direction
	if direction == "desc" {
		result = -result
	}

	return result
}

// Helper functions for sorting
func (a *App) extractHost(urlStr string) string {
	if parsed, err := url.Parse(urlStr); err == nil {
		return parsed.Host
	}
	return ""
}

func (a *App) hasParams(tx network.HTTPTransaction) bool {
	// Check URL parameters
	if parsed, err := url.Parse(tx.Request.URL()); err == nil {
		if len(parsed.Query()) > 0 {
			return true
		}
	}

	// Check for POST parameters
	method := tx.Request.Method()
	return method == "POST" || method == "PUT" || method == "PATCH"
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
	if a.dirtyProjectMeta {
		dirty = append(dirty, "project_meta")
	}
	return dirty
}

// hasDirtyComponents checks if any components are dirty
func (a *App) hasDirtyComponents() bool {
	a.dirtyStateMutex.RLock()
	defer a.dirtyStateMutex.RUnlock()

	return a.dirtyRequestHistory || a.dirtyGleipFlows || a.dirtyAPICollections || a.dirtyProjectMeta
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

// GetAvailableChefActions returns all available chef actions
func (a *App) GetAvailableChefActions() []map[string]string {
	return chef.GetAvailableActions()
}

// GetChefActionPreview generates a preview for a chef action
func (a *App) GetChefActionPreview(actionType string, input string, options map[string]interface{}) string {
	action := chef.ChefAction{
		ActionType: actionType,
		Options:    options,
	}
	return chef.GetPreview(action, input)
}

// GetChefStepSequentialPreview generates previews for a sequence of chef actions
func (a *App) GetChefStepSequentialPreview(actions []chef.ChefAction, inputValue string) ([]string, error) {
	if len(actions) == 0 {
		return []string{}, nil
	}

	return chef.GetAllSequentialPreviews(actions, inputValue)
}

// GetAvailableVariablesForStep returns all available variables for a given step in a gleipflow
func (a *App) GetAvailableVariablesForStep(gleipFlowID string, stepIndex int) ([]string, error) {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	return a.gleipFlowExecutor.GetAvailableVariablesForStep(gleipFlow, stepIndex), nil
}

// GetAvailableVariableValuesForStep returns all available variables and their values for a given step in a gleipflow
func (a *App) GetAvailableVariableValuesForStep(gleipFlowID string, stepIndex int) (map[string]string, error) {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}
	return a.gleipFlowExecutor.GetAvailableVariableValuesForStep(gleipFlow, stepIndex), nil
}

// AddChefAction adds a new action to a chef step
func (a *App) AddChefAction(gleipFlowID string, stepIndex int) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &gleipFlow.Steps[stepIndex]
	if step.StepType != "chef" || step.ChefStep == nil {
		return fmt.Errorf("step at index %d is not a chef step", stepIndex)
	}

	// Create new action
	newAction := chef.ChefAction{
		ID:         fmt.Sprintf("action_%d", time.Now().UnixNano()),
		ActionType: "",
		Options:    make(map[string]interface{}),
		Preview:    "",
	}

	// Add action to the step
	step.ChefStep.Actions = append(step.ChefStep.Actions, newAction)

	// Update and persist the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// RemoveChefAction removes an action from a chef step
func (a *App) RemoveChefAction(gleipFlowID string, stepIndex int, actionIndex int) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &gleipFlow.Steps[stepIndex]
	if step.StepType != "chef" || step.ChefStep == nil {
		return fmt.Errorf("step at index %d is not a chef step", stepIndex)
	}

	if actionIndex < 0 || actionIndex >= len(step.ChefStep.Actions) {
		return fmt.Errorf("invalid action index: %d", actionIndex)
	}

	// Remove action from slice
	step.ChefStep.Actions = append(
		step.ChefStep.Actions[:actionIndex],
		step.ChefStep.Actions[actionIndex+1:]...,
	)

	// Update and persist the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// UpdateChefAction updates an existing action in a chef step
func (a *App) UpdateChefAction(gleipFlowID string, stepIndex int, actionIndex int, action chef.ChefAction) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &gleipFlow.Steps[stepIndex]
	if step.StepType != "chef" || step.ChefStep == nil {
		return fmt.Errorf("step at index %d is not a chef step", stepIndex)
	}

	if actionIndex < 0 || actionIndex >= len(step.ChefStep.Actions) {
		return fmt.Errorf("invalid action index: %d", actionIndex)
	}

	// Update the action
	step.ChefStep.Actions[actionIndex] = action

	// Update and persist the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// UpdateChefStep updates a chef step's properties
func (a *App) UpdateChefStep(gleipFlowID string, stepIndex int, inputVariable string, outputVariable string, name string) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &gleipFlow.Steps[stepIndex]
	if step.StepType != "chef" || step.ChefStep == nil {
		return fmt.Errorf("step at index %d is not a chef step", stepIndex)
	}

	// Update the chef step properties
	if name != "" {
		step.ChefStep.StepAttributes.Name = name
	}
	step.ChefStep.InputVariable = inputVariable
	step.ChefStep.OutputVariable = outputVariable

	// Update and persist the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// UpdateStepExpansion updates the expansion state of a step
func (a *App) UpdateStepExpansion(gleipFlowID string, stepIndex int, isExpanded bool) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Handle variables step (stepIndex -1 indicates variables step)
	if stepIndex == -1 {
		gleipFlow.IsVariableStepExpanded = isExpanded
		return a.UpdateGleipFlow(*gleipFlow)
	}

	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index: %d", stepIndex)
	}

	step := &gleipFlow.Steps[stepIndex]

	// Update the expansion state based on step type
	switch step.StepType {
	case "request":
		if step.RequestStep != nil {
			step.RequestStep.StepAttributes.IsExpanded = isExpanded
		}
	case "script":
		if step.ScriptStep != nil {
			step.ScriptStep.StepAttributes.IsExpanded = isExpanded
		}
	case "chef":
		if step.ChefStep != nil {
			step.ChefStep.StepAttributes.IsExpanded = isExpanded
		}
	default:
		return fmt.Errorf("cannot update expansion for step type: %s", step.StepType)
	}

	// Update and persist the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// PasteRequestToGleipFlowAtPosition pastes a request from clipboard to a specific position in a GleipFlow
func (a *App) PasteRequestToGleipFlowAtPosition(gleipFlowID string, position int) (*GleipFlowStep, error) {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Validate position
	if position < -1 || position > len(gleipFlow.Steps) {
		return nil, fmt.Errorf("invalid position %d for inserting step (valid range: 0-%d or -1 for end)", position, len(gleipFlow.Steps))
	}

	// Get clipboard content
	clipboardText, err := rt.ClipboardGetText(a.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get clipboard text: %v", err)
	}

	if strings.TrimSpace(clipboardText) == "" {
		return nil, fmt.Errorf("clipboard is empty")
	}

	// Try to parse as JSON first (from our own copy operations)
	var httpRequest network.HTTPRequest
	if err := json.Unmarshal([]byte(clipboardText), &httpRequest); err == nil {
		// Successfully parsed as JSON - validate it has required fields
		if httpRequest.Host == "" || httpRequest.Dump == "" {
			return nil, fmt.Errorf("invalid JSON format: missing required fields (host, dump)")
		}
		// Use it directly
	} else {
		// Try to parse as raw HTTP request
		lines := strings.Split(clipboardText, "\n")
		if len(lines) == 0 {
			return nil, fmt.Errorf("invalid request format: empty content")
		}

		// Check if first line looks like an HTTP request line
		firstLine := strings.TrimSpace(lines[0])
		if !strings.Contains(firstLine, "HTTP/") {
			return nil, fmt.Errorf("invalid request format: doesn't appear to be an HTTP request")
		}

		// Parse the request line to validate format
		parts := strings.Fields(firstLine)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid HTTP request line format")
		}

		// Extract host from Host header (required for raw HTTP requests)
		var host string
		for _, line := range lines[1:] {
			trimmedLine := strings.TrimSpace(line)
			if strings.HasPrefix(strings.ToLower(trimmedLine), "host:") {
				// Extract host value after "Host:"
				hostParts := strings.SplitN(trimmedLine, ":", 2)
				if len(hostParts) == 2 {
					host = strings.TrimSpace(hostParts[1])
				}
				break
			}
		}

		if host == "" {
			return nil, fmt.Errorf("invalid request format: Host header not found")
		}

		// Create HTTP request with TLS=true as specified
		httpRequest = network.HTTPRequest{
			Host: host,
			Dump: clipboardText,
			TLS:  true, // Always assume TLS=true for raw HTTP requests as specified
		}
	}

	// Count existing request steps for naming
	requestCount := 0
	for _, step := range gleipFlow.Steps {
		if step.StepType == "request" {
			requestCount++
		}
	}

	// Create new request step
	newStep := GleipFlowStep{
		StepType: "request",
		Selected: true,
		RequestStep: &RequestStep{
			StepAttributes: gleipflow.StepAttributes{
				ID:         uuid.New().String(),
				Name:       fmt.Sprintf("Request %d", requestCount+1),
				IsExpanded: true,
			},
			Request:                  httpRequest,
			VariableExtracts:         []VariableExtract{},
			RecalculateContentLength: true,
			GunzipResponse:           true,
			CameFrom:                 "clipboard",
		},
	}

	// Insert step at the specified position
	if position == -1 || position == len(gleipFlow.Steps) {
		// Add to the end
		gleipFlow.Steps = append(gleipFlow.Steps, newStep)
	} else {
		// Insert at specific position
		gleipFlow.Steps = append(gleipFlow.Steps[:position], append([]GleipFlowStep{newStep}, gleipFlow.Steps[position:]...)...)
	}

	// Track step addition
	TrackFlowStepExecuted(gleipFlowID, "request", true)

	// Update and persist the flow
	err = a.UpdateGleipFlow(*gleipFlow)
	if err != nil {
		return nil, fmt.Errorf("failed to save gleipFlow: %v", err)
	}

	return &newStep, nil
}

// UpdateGleipFlow updates a gleipFlow and automatically saves
func (a *App) UpdateGleipFlow(gleipFlow GleipFlow) error {
	fmt.Printf("DEBUG: UpdateGleipFlow called with flow ID: %s, name: %s\n", gleipFlow.ID, gleipFlow.Name)

	// Generate ID if not present
	if gleipFlow.ID == "" {
		gleipFlow.ID = uuid.New().String()
		fmt.Printf("DEBUG: Generated new ID for flow: %s\n", gleipFlow.ID)
		TrackFlowCreated(gleipFlow.ID, "custom")
	}

	// Ensure sortingIndex is valid (>= 1)
	if gleipFlow.SortingIndex < 1 {
		// Find the highest sorting index and add 1
		highestIndex := 0
		for _, g := range a.gleipFlowsCache {
			if g.SortingIndex > highestIndex {
				highestIndex = g.SortingIndex
			}
		}
		gleipFlow.SortingIndex = highestIndex + 1
	}

	a.gleipFlowsMutex.Lock()
	// ALWAYS preserve execution results - UpdateGleipFlow should never touch them
	if existingFlow, exists := a.gleipFlowsCache[gleipFlow.ID]; exists {
		gleipFlow.ExecutionResults = existingFlow.ExecutionResults
		fmt.Printf("DEBUG: Preserved %d execution results from cache\n", len(gleipFlow.ExecutionResults))
	} else {
		fmt.Printf("DEBUG: No existing flow in cache, execution results: %d\n", len(gleipFlow.ExecutionResults))
	}
	// Update the cache
	a.gleipFlowsCache[gleipFlow.ID] = &gleipFlow
	a.gleipFlowsMutex.Unlock()

	// Also update the project's GleipFlows array to keep them in sync
	a.projectMutex.Lock()
	if a.currentProject != nil {
		// Find and update existing GleipFlow in project, or add new one
		found := false
		for i, projectGleipFlow := range a.currentProject.GleipFlows {
			if projectGleipFlow.ID == gleipFlow.ID {
				// Update existing - execution results already preserved from cache above
				a.currentProject.GleipFlows[i] = &gleipFlow
				found = true
				fmt.Printf("DEBUG: Updated existing project flow\n")
				break
			}
		}
		if !found {
			// Add new GleipFlow to project
			a.currentProject.GleipFlows = append(a.currentProject.GleipFlows, &gleipFlow)

		}
	}
	a.projectMutex.Unlock()

	// AUTOMATICALLY save to persist the project changes
	a.requestAutoSaveWithComponent("gleip_flows")

	return nil
}

// updateChefStepActionPreviews updates action previews for chef steps that use the given variables
func (a *App) updateChefStepActionPreviews(gleipFlow *GleipFlow, updatedVarNames map[string]bool) {
	for stepIndex, step := range gleipFlow.Steps {
		if step.StepType == "chef" && step.Selected && step.ChefStep != nil {
			inputVar := step.ChefStep.InputVariable
			if inputVar != "" && updatedVarNames[inputVar] && gleipFlow.Variables[inputVar] != "" {
				inputValue := gleipFlow.Variables[inputVar]

				if len(step.ChefStep.Actions) > 0 {
					previews, err := chef.GetAllSequentialPreviews(step.ChefStep.Actions, inputValue)
					if err == nil {
						for i, preview := range previews {
							if i < len(step.ChefStep.Actions) {
								gleipFlow.Steps[stepIndex].ChefStep.Actions[i].Preview = preview
							}
						}
					}
				}
			}
		}
	}
}

// UpdateGleipFlowVariables updates variables in a gleipFlow and automatically executes all enabled chef steps
func (a *App) UpdateGleipFlowVariables(gleipFlowID string, variables map[string]string) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Filter out empty variable names and track what variables changed
	filteredVariables := make(map[string]string)
	changedVars := make(map[string]string)

	for varName, newValue := range variables {
		// Skip empty variable names
		trimmedName := strings.TrimSpace(varName)
		if trimmedName == "" {
			continue
		}

		filteredVariables[trimmedName] = newValue

		oldValue, exists := gleipFlow.Variables[trimmedName]
		if !exists || oldValue != newValue {
			changedVars[trimmedName] = newValue
		}
	}

	// Completely replace the variables map to handle deletions properly
	gleipFlow.Variables = make(map[string]string)
	for varName, value := range filteredVariables {
		gleipFlow.Variables[varName] = value
	}

	// Update action previews for all enabled chef steps immediately after variable change
	if len(changedVars) > 0 {
		// Convert changedVars to a map[string]bool for the helper function
		changedVarNames := make(map[string]bool)
		for varName := range changedVars {
			changedVarNames[varName] = true
		}

		// Update action previews for chef steps that use the changed variables
		a.updateChefStepActionPreviews(gleipFlow, changedVarNames)

		// Then execute chef steps and persist everything
		err = a.executeEnabledChefSteps(gleipFlow, changedVars)
		if err != nil {
			// Don't fail the variable update if chef execution fails
		}
		// executeEnabledChefSteps already handles persistence, so we're done
		return nil
	}

	// If no chef steps were executed, just update the variables in the flow
	return a.UpdateGleipFlow(*gleipFlow)
}

// executeEnabledChefSteps executes enabled (selected) chef steps while preventing infinite loops
func (a *App) executeEnabledChefSteps(gleipFlow *GleipFlow, initiallyChangedVars map[string]string) error {
	// Create execution context with current variables
	ctx := NewExecutionContext()
	ctx.Variables = make(map[string]string)
	for k, v := range gleipFlow.Variables {
		ctx.Variables[k] = v
	}

	// Track which chef steps have been executed to prevent infinite loops
	executedSteps := make(map[string]bool)
	// Track which variables have been changed (initially + by chef steps)
	changedVariables := make(map[string]bool)
	for varName := range initiallyChangedVars {
		changedVariables[varName] = true
	}

	// Keep executing chef steps until no new variables are created
	hasExecutedSteps := false
	maxIterations := 10 // Prevent infinite loops
	for iteration := 0; iteration < maxIterations; iteration++ {
		stepExecutedThisIteration := false

		for stepIndex, step := range gleipFlow.Steps {
			if step.StepType == "chef" && step.Selected && step.ChefStep != nil {
				stepID := step.ChefStep.StepAttributes.ID
				inputVar := step.ChefStep.InputVariable

				// Skip if already executed
				if executedSteps[stepID] {
					continue
				}

				// Skip if no input variable specified
				if inputVar == "" {
					continue
				}

				// Only execute if the input variable has been changed
				if !changedVariables[inputVar] {
					continue
				}

				// Execute the chef step (action previews already updated earlier)
				result := a.gleipFlowExecutor.ExecuteChefStep(step.ChefStep, ctx)

				// Mark this step as executed to prevent re-execution
				executedSteps[stepID] = true
				stepExecutedThisIteration = true
				hasExecutedSteps = true

				// Update execution results in the flow
				if gleipFlow.ExecutionResults == nil {
					gleipFlow.ExecutionResults = []ExecutionResult{}
				}

				// Remove any existing result for this step
				filteredResults := make([]ExecutionResult, 0)
				for _, existingResult := range gleipFlow.ExecutionResults {
					if existingResult.StepID != result.StepID {
						filteredResults = append(filteredResults, existingResult)
					}
				}
				// Add the new result
				filteredResults = append(filteredResults, result)
				gleipFlow.ExecutionResults = filteredResults

				// If chef step produced output variables, merge them back into flow variables
				if result.Success && result.Variables != nil {
					for varName, varValue := range result.Variables {
						gleipFlow.Variables[varName] = varValue
						ctx.Variables[varName] = varValue // Also update the execution context
						changedVariables[varName] = true  // Mark as changed so other chef steps can use it

					}
				}

				// Emit step execution event for immediate UI updates
				if a.gleipFlowExecutor.eventEmitter != nil {
					a.gleipFlowExecutor.eventEmitter.EmitStepExecuted(gleipFlow.ID, stepIndex, filteredResults)
				}
			}
		}

		// If no steps were executed this iteration, we're done
		if !stepExecutedThisIteration {
			break
		}
	}

	if hasExecutedSteps {
		// Update cached flow with new execution results, variables, and updated chef step action previews
		a.gleipFlowsMutex.Lock()
		if cachedFlow, exists := a.gleipFlowsCache[gleipFlow.ID]; exists {
			cachedFlow.ExecutionResults = gleipFlow.ExecutionResults
			cachedFlow.Variables = gleipFlow.Variables
			cachedFlow.Steps = gleipFlow.Steps // Update entire steps array to include action preview changes
		}
		a.gleipFlowsMutex.Unlock()

		// Update project flow with new execution results, variables, and updated chef step action previews
		a.projectMutex.Lock()
		if a.currentProject != nil {
			for _, projectFlow := range a.currentProject.GleipFlows {
				if projectFlow.ID == gleipFlow.ID {
					projectFlow.ExecutionResults = gleipFlow.ExecutionResults
					projectFlow.Variables = gleipFlow.Variables
					projectFlow.Steps = gleipFlow.Steps // Update entire steps array to include action preview changes
					break
				}
			}
		}
		a.projectMutex.Unlock()

		// Request auto-save since we've modified gleipFlow data
		a.requestAutoSaveWithComponent("gleip_flows")
	}

	return nil
}

// mergeExecutionResults merges new execution results with existing ones
// Only updates results for steps that were actually executed (have new results)
// Preserves existing results for steps that weren't executed
func (a *App) mergeExecutionResults(existingResults []ExecutionResult, newResults []ExecutionResult) []ExecutionResult {
	if len(newResults) == 0 {
		return existingResults
	}

	// Create a map of existing results by step ID for fast lookup
	existingByStepID := make(map[string]ExecutionResult)
	for _, result := range existingResults {
		existingByStepID[result.StepID] = result
	}

	// Update with new results (overwrite existing or add new)
	for _, newResult := range newResults {
		existingByStepID[newResult.StepID] = newResult
	}

	// Convert back to slice
	mergedResults := make([]ExecutionResult, 0, len(existingByStepID))
	for _, result := range existingByStepID {
		mergedResults = append(mergedResults, result)
	}

	return mergedResults
}

// applyRequestFilters applies the advanced filters to the transactions
func (a *App) applyRequestFilters(transactions []network.HTTPTransaction, filters RequestFilters) []network.HTTPTransaction {
	var filtered []network.HTTPTransaction

	for _, tx := range transactions {
		if a.transactionMatchesFilters(tx, filters) {
			filtered = append(filtered, tx)
		}
	}

	return filtered
}

// transactionMatchesFilters checks if a transaction matches the given filters
func (a *App) transactionMatchesFilters(tx network.HTTPTransaction, filters RequestFilters) bool {
	// Filter by parameters
	if filters.HasParams != "" {
		hasParams := a.hasParams(tx)
		if (filters.HasParams == "yes" && !hasParams) || (filters.HasParams == "no" && hasParams) {
			return false
		}
	}

	// Filter by status codes
	if filters.StatusCodes != "" {
		if tx.Response == nil {
			return false
		}
		statusCode := tx.Response.StatusCode()
		if !a.matchesStatusCodeFilter(statusCode, filters.StatusCodes) {
			return false
		}
	}

	// Filter by methods
	if len(filters.Methods) > 0 {
		method := tx.Request.Method()
		methodMatches := false
		for _, filterMethod := range filters.Methods {
			if strings.EqualFold(method, filterMethod) {
				methodMatches = true
				break
			}
		}
		if !methodMatches {
			return false
		}
	}

	// Filter by response size
	if filters.ResponseSize.Value != "" {
		if tx.Response == nil {
			return false
		}
		responseSize := len(tx.Response.Dump)
		filterValue, err := strconv.Atoi(filters.ResponseSize.Value)
		if err != nil {
			return false
		}

		switch filters.ResponseSize.Operator {
		case ">":
			if responseSize <= filterValue {
				return false
			}
		case "<":
			if responseSize >= filterValue {
				return false
			}
		}
	}

	// Filter by hosts
	if filters.Hosts != "" {
		host := a.extractHost(tx.Request.URL())
		if !a.matchesHostFilter(host, filters.Hosts) {
			return false
		}
	}

	return true
}

// matchesStatusCodeFilter checks if a status code matches the filter string
func (a *App) matchesStatusCodeFilter(statusCode int, filter string) bool {
	parts := strings.Split(filter, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for range (e.g., "200-299")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) == 2 {
				start, err1 := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
				end, err2 := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
				if err1 == nil && err2 == nil {
					if statusCode >= start && statusCode <= end {
						return true
					}
				}
			}
		} else {
			// Check for exact match
			if code, err := strconv.Atoi(part); err == nil {
				if statusCode == code {
					return true
				}
			}
		}
	}
	return false
}

// matchesHostFilter checks if a host matches the filter string
func (a *App) matchesHostFilter(host string, filter string) bool {
	hostFilters := strings.Split(filter, ",")
	for _, hostFilter := range hostFilters {
		hostFilter = strings.TrimSpace(hostFilter)
		if hostFilter == "" {
			continue
		}

		// Contains matching
		if strings.Contains(strings.ToLower(host), strings.ToLower(hostFilter)) {
			return true
		}
	}
	return false
}
