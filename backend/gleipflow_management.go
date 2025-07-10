package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/gleipflow"
	"Gleip/backend/network"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Global cancellation variable for the fuzzing process
var fuzzCancellation = make(chan struct{})
var fuzzMutex sync.Mutex

// Global phantom request rate limiting state
var phantomRequestState = struct {
	mutex                 sync.Mutex
	lastRefreshTime       map[string]time.Time // gleipFlowID -> last refresh time
	lastRequestSignature  map[string]string    // gleipFlowID -> last request signature
	activeFuzzingSessions map[string]bool      // gleipFlowID -> fuzzing active
}{
	lastRefreshTime:       make(map[string]time.Time),
	lastRequestSignature:  make(map[string]string),
	activeFuzzingSessions: make(map[string]bool),
}

const (
	phantomRefreshCooldown = 5 * time.Second // 5 seconds minimum between refreshes
)

// SaveGleipFlow saves or updates a gleipFlow
func (a *App) SaveGleipFlow(gleipFlow GleipFlow) (GleipFlow, error) {
	fmt.Printf("DEBUG: SaveGleipFlow called with flow ID: %s, name: %s\n", gleipFlow.ID, gleipFlow.Name)

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

	// If flow exists, preserve execution results
	if existingFlow, exists := a.gleipFlowsCache[gleipFlow.ID]; exists {
		// Preserve execution results from existing flow
		gleipFlow.ExecutionResults = existingFlow.ExecutionResults
		fmt.Printf("DEBUG: Preserved %d execution results\n", len(gleipFlow.ExecutionResults))
	}

	// Update the cache
	a.gleipFlowsCache[gleipFlow.ID] = &gleipFlow
	fmt.Printf("DEBUG: Updated cache for flow: %s\n", gleipFlow.ID)

	a.gleipFlowsMutex.Unlock()

	// Also update the project's GleipFlows array to keep them in sync
	a.projectMutex.Lock()
	if a.currentProject != nil {
		// Find and update existing GleipFlow in project, or add new one
		found := false
		for i, projectGleipFlow := range a.currentProject.GleipFlows {
			if projectGleipFlow.ID == gleipFlow.ID {
				// Preserve execution results when updating project data
				gleipFlow.ExecutionResults = projectGleipFlow.ExecutionResults
				a.currentProject.GleipFlows[i] = &gleipFlow
				found = true
				fmt.Printf("DEBUG: Updated project flow at index %d\n", i)
				break
			}
		}

		if !found {
			// Add new GleipFlow to the project
			a.currentProject.GleipFlows = append(a.currentProject.GleipFlows, &gleipFlow)
			fmt.Printf("DEBUG: Added new flow to project\n")
		}
	}

	a.projectMutex.Unlock()

	// Request auto-save to persist the project changes
	fmt.Printf("DEBUG: Requesting auto-save with component 'gleip_flows'\n")
	a.requestAutoSaveWithComponent("gleip_flows")

	return gleipFlow, nil
}

// StartFuzzing starts a fuzzing session on a specific request step
func (a *App) StartFuzzing(gleipFlowID string, stepID string) error {
	// Get the flow
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		// Track error with metadata approach
		TrackError("fuzzing", "flow_not_found")
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Find the request step and its index
	var requestStep *RequestStep
	var targetStepIndex int = -1
	for i, step := range gleipFlow.Steps {
		if step.StepType == "request" && step.RequestStep != nil && step.RequestStep.StepAttributes.ID == stepID {
			requestStep = step.RequestStep
			targetStepIndex = i
			break
		}
	}

	if requestStep == nil {
		// Track error with metadata approach
		TrackError("fuzzing", "step_not_found")
		return fmt.Errorf("step not found: %s", stepID)
	}

	// Verify fuzz settings are available
	if requestStep.FuzzSettings == nil || len(requestStep.FuzzSettings.CurrentWordlist) == 0 {
		// Track error with metadata approach
		TrackError("fuzzing", "no_wordlist")
		return fmt.Errorf("no wordlist available for fuzzing")
	}

	// Track fuzzing configuration
	TrackFuzzingStarted(gleipFlowID, stepID, len(requestStep.FuzzSettings.CurrentWordlist))

	// Clear previous fuzz results only when explicitly starting a new fuzzing session
	requestStep.FuzzSettings.FuzzResults = []FuzzResult{}
	fmt.Printf("Cleared previous fuzz results for step %s\n", stepID)

	// Mark this flow as having an active fuzzing session
	phantomRequestState.mutex.Lock()
	phantomRequestState.activeFuzzingSessions[gleipFlowID] = true
	phantomRequestState.mutex.Unlock()

	// Reset the cancellation channel
	fuzzCancellation = make(chan struct{})

	// Create a copy of the flow with modified selections for execution (similar to ExecuteSingleStep)
	flowCopy := *gleipFlow
	flowCopy.Steps = make([]GleipFlowStep, len(gleipFlow.Steps))
	copy(flowCopy.Steps, gleipFlow.Steps)

	// Modify selections in the copy: only target request step + selected non-request steps
	for i := range flowCopy.Steps {
		if i == targetStepIndex {
			// Always select the target request step
			flowCopy.Steps[i].Selected = true
		} else if flowCopy.Steps[i].StepType == "request" {
			// Deselect all other request steps
			flowCopy.Steps[i].Selected = false
		}
		// Non-request steps keep their original selection for context
	}

	// Execute the flow with the modified selections
	_, err = a.gleipFlowExecutor.ExecuteGleipFlow(&flowCopy)
	if err != nil {
		// Track error
		TrackError("fuzzing", "execution_error")
		return fmt.Errorf("failed to execute fuzz: %v", err)
	}

	// Save back the fuzz results while preserving original Selected properties
	// Update the cached flow with fuzz results but preserve original Selected properties
	a.gleipFlowsMutex.Lock()
	if cachedFlow, exists := a.gleipFlowsCache[gleipFlowID]; exists {
		// Update steps array but preserve original Selected properties
		for i := range cachedFlow.Steps {
			if i < len(flowCopy.Steps) {
				// Preserve original Selected property
				originalSelected := cachedFlow.Steps[i].Selected
				cachedFlow.Steps[i] = flowCopy.Steps[i]
				cachedFlow.Steps[i].Selected = originalSelected
			}
		}
	}
	a.gleipFlowsMutex.Unlock()

	// Update the project flow with fuzz results but preserve original Selected properties
	a.projectMutex.Lock()
	if a.currentProject != nil {
		for _, projectFlow := range a.currentProject.GleipFlows {
			if projectFlow.ID == gleipFlowID {
				// Update steps array but preserve original Selected properties
				for i := range projectFlow.Steps {
					if i < len(flowCopy.Steps) {
						// Preserve original Selected property
						originalSelected := projectFlow.Steps[i].Selected
						projectFlow.Steps[i] = flowCopy.Steps[i]
						projectFlow.Steps[i].Selected = originalSelected
					}
				}
				break
			}
		}
	}
	a.projectMutex.Unlock()

	// Request auto-save since we've modified gleipFlow data (including fuzz results)
	a.requestAutoSaveWithComponent("gleip_flows")

	// Mark fuzzing as completed for this flow
	phantomRequestState.mutex.Lock()
	phantomRequestState.activeFuzzingSessions[gleipFlowID] = false
	phantomRequestState.mutex.Unlock()

	return nil
}

// StopFuzzing stops a fuzzing session that is currently in progress
func (a *App) StopFuzzing() error {
	// Log the cancellation attempt
	fmt.Printf("Stopping fuzzing operation...\n")

	// Track fuzzing stop
	TrackFuzzingCompleted("", "", 0, true)

	// Create a mutex to safely handle the cancellation channel
	fuzzMutex.Lock()
	defer fuzzMutex.Unlock()

	// Check if the channel is already closed
	select {
	case <-fuzzCancellation:
		// Channel is already closed, create a new one
		fmt.Printf("Cancellation channel was already closed, creating a new one\n")
		fuzzCancellation = make(chan struct{})
	default:
		// Channel is still open, close it to signal cancellation
		close(fuzzCancellation)
		fmt.Printf("Closed cancellation channel to stop fuzzing\n")
	}

	// Create a new channel for the next fuzzing operation
	fuzzCancellation = make(chan struct{})

	// Clear all active fuzzing sessions since we're stopping all fuzzing
	phantomRequestState.mutex.Lock()
	for flowID := range phantomRequestState.activeFuzzingSessions {
		phantomRequestState.activeFuzzingSessions[flowID] = false
	}
	phantomRequestState.mutex.Unlock()

	fmt.Printf("Fuzzing stopped by user\n")
	return nil
}

// GetGleipFlows returns all saved request gleipFlows
func (a *App) GetGleipFlows() []GleipFlow {
	// Get flows from project data (most reliable source)
	a.projectMutex.RLock()
	var gleipFlows []GleipFlow
	if a.currentProject != nil {
		gleipFlows = make([]GleipFlow, len(a.currentProject.GleipFlows))
		for i, flow := range a.currentProject.GleipFlows {
			gleipFlows[i] = *flow

			// Recreate HTTPRequest objects to ensure Go methods are available
			for j, step := range gleipFlows[i].Steps {
				if step.StepType == "request" && step.RequestStep != nil {
					// Create a fresh HTTPRequest object with proper method bindings
					freshRequest := network.HTTPRequest{
						Host: step.RequestStep.Request.Host,
						TLS:  step.RequestStep.Request.TLS,
						Dump: step.RequestStep.Request.Dump,
					}
					// Replace with the fresh object that has Go methods
					gleipFlows[i].Steps[j].RequestStep.Request = freshRequest
				}
			}
		}
	}
	a.projectMutex.RUnlock()

	return gleipFlows
}

// GetGleipFlow returns a specific request gleipFlow by ID
func (a *App) GetGleipFlow(id string) (*GleipFlow, error) {
	if id == "" {
		return nil, fmt.Errorf("gleipFlow ID cannot be empty")
	}

	// First try the cache
	a.gleipFlowsMutex.RLock()
	gleipFlow, exists := a.gleipFlowsCache[id]
	a.gleipFlowsMutex.RUnlock()

	if exists {
		return gleipFlow, nil
	}

	// If not in cache, try to find it in the project data and update cache
	a.projectMutex.RLock()
	var foundFlow *GleipFlow
	if a.currentProject != nil {
		for _, flow := range a.currentProject.GleipFlows {
			if flow.ID == id {
				foundFlow = flow
				break
			}
		}
	}
	a.projectMutex.RUnlock()

	if foundFlow != nil {
		// Add to cache
		a.gleipFlowsMutex.Lock()
		a.gleipFlowsCache[id] = foundFlow
		a.gleipFlowsMutex.Unlock()
		return foundFlow, nil
	}

	return nil, fmt.Errorf("gleipFlow not found: %s", id)
}

// DeleteGleipFlow deletes a request gleipFlow
func (a *App) DeleteGleipFlow(id string) error {
	a.gleipFlowsMutex.Lock()
	// Check if gleipFlow exists
	_, exists := a.gleipFlowsCache[id]
	if !exists {
		a.gleipFlowsMutex.Unlock()
		return fmt.Errorf("gleipFlow not found: %s", id)
	}

	// Track deletion
	TrackFlowDeleted(id)

	// Delete from cache
	delete(a.gleipFlowsCache, id)
	a.gleipFlowsMutex.Unlock()

	// Also remove from project's GleipFlows array
	a.projectMutex.Lock()
	if a.currentProject != nil {
		// Find and remove the GleipFlow from project
		for i, projectGleipFlow := range a.currentProject.GleipFlows {
			if projectGleipFlow.ID == id {
				// Remove from slice
				a.currentProject.GleipFlows = append(
					a.currentProject.GleipFlows[:i],
					a.currentProject.GleipFlows[i+1:]...,
				)
				break
			}
		}
	}
	a.projectMutex.Unlock()

	// Request auto-save to persist the project changes
	a.requestAutoSaveWithComponent("gleip_flows")

	return nil
}

// CopyRequestToClipboard copies a request to the system clipboard for use in the gleipFlow
func (a *App) CopyRequestToClipboard(id string) error {
	// Get the full transaction details
	transaction, err := a.proxyServer.GetTransactionDetails(id)
	if err != nil {
		// Track error
		TrackError("clipboard", "get_transaction_error")
		return fmt.Errorf("failed to get transaction details: %v", err)
	}

	// Track request copying to clipboard from history
	TrackRequestCopiedToClipboard(transaction.Request.Method(), transaction.Request.URL(), "history")

	// Determine if TLS is being used
	isTLS := strings.HasPrefix(transaction.Request.URL(), "https://")

	// Create HTTPRequest object in the proper format (transaction.Request is already HTTPRequest)
	httpRequest := network.HTTPRequest{
		Host: transaction.Request.Host,
		Dump: transaction.Request.Dump,
		TLS:  isTLS,
	}

	// Serialize to JSON for the clipboard
	serialized, err := json.MarshalIndent(httpRequest, "", "  ")
	if err != nil {
		// Track error
		TrackError("clipboard", "serialize_request_error")
		return fmt.Errorf("failed to serialize request: %v", err)
	}

	// Set to system clipboard using the wails runtime
	runtime.ClipboardSetText(a.ctx, string(serialized))

	return nil
}

// CopyRequestToCurrentFlow copies a request directly to the currently selected flow
// This function handles both proxy requests (from history) and API collection requests
func (a *App) CopyRequestToCurrentFlow(requestID string, collectionID ...string) error {
	// Get all flows first
	gleipFlows := a.GetGleipFlows()
	if len(gleipFlows) == 0 {
		return fmt.Errorf("no flows are available to copy the request to")
	}

	// Get the selected flow ID from the project
	a.projectMutex.RLock()
	selectedFlowID := ""
	if a.currentProject != nil {
		selectedFlowID = a.currentProject.SelectedGleipFlowID
	}
	a.projectMutex.RUnlock()

	// Find the selected flow in our list
	var targetFlow *GleipFlow
	for i := range gleipFlows {
		if gleipFlows[i].ID == selectedFlowID {
			targetFlow = &gleipFlows[i]
			break
		}
	}

	// If no flow is selected or the selected flow doesn't exist, use the first one
	if targetFlow == nil {
		// Sort by sorting index and select the first one
		targetFlow = &gleipFlows[0]
		for i := range gleipFlows {
			if gleipFlows[i].SortingIndex < targetFlow.SortingIndex {
				targetFlow = &gleipFlows[i]
			}
		}

		// Update the selected flow
		a.SetSelectedGleipFlowID(targetFlow.ID)
	}

	// Determine if this is an API collection request or proxy request
	isAPIRequest := len(collectionID) > 0 && collectionID[0] != ""

	var newStep GleipFlowStep
	var trackingURL string
	var trackingMethod string
	var trackingSource string
	var err error // Declare err at function level

	if isAPIRequest {
		// Handle API collection request
		collection, err := a.GetAPICollection(collectionID[0])
		if err != nil {
			return fmt.Errorf("API collection not found: %v", err)
		}

		// Find the request
		var apiRequest *APIRequest
		for i := range collection.Requests {
			if collection.Requests[i].ID == requestID {
				apiRequest = &collection.Requests[i]
				break
			}
		}

		if apiRequest == nil {
			return fmt.Errorf("API request not found: %s", requestID)
		}

		// Apply security if there's an active security scheme
		var rawRequest string

		// Check if there's an active security scheme
		if collection.ActiveSecurity != "" {
			// Always try to get the request with security
			req, err := a.GetRequestWithSecurity(collectionID[0], requestID)
			if err == nil {
				rawRequest = req
			}
		}

		// If we couldn't get the request with security, build it without security
		if rawRequest == "" {
			// Build a raw HTTP request
			method := apiRequest.Method
			path := apiRequest.Path
			var headerLines []string

			// Add headers from the API request (Host header should already be included)
			for _, header := range apiRequest.Headers {
				headerLines = append(headerLines, fmt.Sprintf("%s: %s", header.Name, header.Value))
			}

			// Build the raw request
			rawRequest = fmt.Sprintf("%s %s HTTP/1.1\r\n%s\r\n\r\n%s",
				method,
				path,
				strings.Join(headerLines, "\r\n"),
				apiRequest.Body)
		}

		// Count existing request steps
		requestCount := 0
		for _, step := range targetFlow.Steps {
			if step.StepType == "request" {
				requestCount++
			}
		}

		// Create a new request step
		newStep = GleipFlowStep{
			StepType: "request",
			Selected: true,
			RequestStep: &RequestStep{
				StepAttributes: gleipflow.StepAttributes{
					ID:         uuid.New().String(),
					Name:       fmt.Sprintf("Request %d", requestCount+1),
					IsExpanded: true,
				},
				Request: network.HTTPRequest{
					Host: apiRequest.Host,
					Dump: rawRequest,
					TLS:  strings.HasPrefix(strings.ToLower(apiRequest.URL), "https://") || strings.HasPrefix(strings.ToLower(trackingURL), "https://"), // Determine TLS from URL
				},
				VariableExtracts:         []VariableExtract{},
				RecalculateContentLength: true,
				GunzipResponse:           true,
				CameFrom:                 "history",
			},
		}

		// Set tracking variables
		trackingMethod = apiRequest.Method
		trackingSource = "api_collection"
	} else {
		// Handle proxy request (from history)
		transaction, err := a.proxyServer.GetTransactionDetails(requestID)
		if err != nil {
			// Track error
			TrackError("clipboard", "get_transaction_error")
			return fmt.Errorf("failed to get transaction details: %v", err)
		}

		// Parse headers from the request dump
		headerLines := strings.Split(transaction.Request.Dump, "\r\n")
		headerEndIndex := -1

		for i, line := range headerLines {
			if line == "" {
				headerEndIndex = i
				break
			}
		}

		if headerEndIndex == -1 {
			// Track error
			TrackError("clipboard", "invalid_request_format")
			return fmt.Errorf("invalid request format: couldn't find end of headers")
		}

		// Extract headers
		headers := make(map[string]string)
		// Skip the first line (HTTP method line) and parse headers
		for i := 1; i < headerEndIndex; i++ {
			line := headerLines[i]
			colonIndex := strings.Index(line, ":")
			if colonIndex > 0 {
				key := strings.TrimSpace(line[:colonIndex])
				value := strings.TrimSpace(line[colonIndex+1:])
				headers[key] = value
			}
		}

		// Count existing request steps
		requestCount := 0
		for _, step := range targetFlow.Steps {
			if step.StepType == "request" {
				requestCount++
			}
		}

		// Create a new request step
		newStep = GleipFlowStep{
			StepType: "request",
			Selected: true,
			RequestStep: &RequestStep{
				StepAttributes: gleipflow.StepAttributes{
					ID:         uuid.New().String(),
					Name:       fmt.Sprintf("Request %d", requestCount+1),
					IsExpanded: true,
				},
				Request:                  transaction.Request,
				VariableExtracts:         []VariableExtract{},
				RecalculateContentLength: true,
				GunzipResponse:           true,
				CameFrom:                 "history",
			},
		}

		// Set tracking variables
		trackingMethod = transaction.Request.Method()
		trackingSource = "history"
	}

	// Add step to flow
	targetFlow.Steps = append(targetFlow.Steps, newStep)

	// Track request copying to flow
	TrackRequestCopiedToFlow(trackingMethod, trackingSource, targetFlow.ID)

	// Track step addition
	TrackFlowStepExecuted(targetFlow.ID, "request", true)

	// AUTOMATICALLY save the flow
	err = a.UpdateGleipFlow(*targetFlow)
	if err != nil {
		// Track error
		TrackError("clipboard", "save_flow_error")
		return fmt.Errorf("failed to save gleipFlow: %v", err)
	}

	return nil
}

// CopyAPIRequestToCurrentFlow is now a wrapper for backward compatibility
func (a *App) CopyAPIRequestToCurrentFlow(collectionID string, requestID string) error {
	return a.CopyRequestToCurrentFlow(requestID, collectionID)
}

// CopyRequestToSelectedFlow is now a wrapper for backward compatibility
func (a *App) CopyRequestToSelectedFlow(id string) error {
	return a.CopyRequestToCurrentFlow(id)
}

// CreateGleipFlow creates a new empty GleipFlow
func (a *App) CreateGleipFlow(name string) (GleipFlow, error) {
	// Generate a name if empty
	if name == "" {
		// Find highest number and increment
		maxNumber := 0
		for _, flow := range a.gleipFlowsCache {
			if strings.HasPrefix(flow.Name, "GleipFlow ") {
				if num, err := strconv.Atoi(strings.TrimPrefix(flow.Name, "GleipFlow ")); err == nil && num > maxNumber {
					maxNumber = num
				}
			}
		}
		name = fmt.Sprintf("GleipFlow %d", maxNumber+1)
	}

	// Create new flow with proper sorting index
	highestIndex := 0
	for _, g := range a.gleipFlowsCache {
		if g.SortingIndex > highestIndex {
			highestIndex = g.SortingIndex
		}
	}

	newFlowID := uuid.New().String()
	fmt.Printf("DEBUG: CreateGleipFlow creating new flow with ID: %s, name: %s\n", newFlowID, name)

	newFlow := GleipFlow{
		ID:           newFlowID,
		Name:         name,
		Variables:    make(map[string]string),
		Steps:        []GleipFlowStep{},
		SortingIndex: highestIndex + 1,
	}

	// Track creation
	TrackFlowCreated(newFlow.ID, "empty")

	// AUTOMATICALLY save the new flow (this will also add it to the project)
	err := a.UpdateGleipFlow(newFlow)
	if err != nil {
		return GleipFlow{}, err
	}

	// Set as the selected GleipFlow in the project
	a.projectMutex.Lock()
	if a.currentProject != nil {
		a.currentProject.SelectedGleipFlowID = newFlow.ID
	}
	a.projectMutex.Unlock()

	// Request auto-save to persist the selection
	a.requestAutoSaveWithComponent("project_meta")

	return newFlow, nil
}

// AddStepToGleipFlow adds a new step to an existing GleipFlow
func (a *App) AddStepToGleipFlow(gleipFlowID string, stepType string) (*GleipFlowStep, error) {
	return a.AddStepToGleipFlowAtPosition(gleipFlowID, stepType, -1)
}

// AddStepToGleipFlowAtPosition adds a new step to an existing GleipFlow at the specified position
// If position is -1, the step is added at the end
func (a *App) AddStepToGleipFlowAtPosition(gleipFlowID string, stepType string, position int) (*GleipFlowStep, error) {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		// Track error
		TrackError("gleipflow", "get_flow_error")
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Validate position
	if position < -1 || position > len(gleipFlow.Steps) {
		return nil, fmt.Errorf("invalid position %d for inserting step (valid range: 0-%d or -1 for end)", position, len(gleipFlow.Steps))
	}

	newStep := GleipFlowStep{
		StepType: stepType,
		Selected: true,
	}

	if stepType == "request" {
		// Count existing request steps
		requestCount := 0
		for _, step := range gleipFlow.Steps {
			if step.StepType == "request" {
				requestCount++
			}
		}

		newStep.RequestStep = &RequestStep{
			StepAttributes: gleipflow.StepAttributes{
				ID:         uuid.New().String(),
				Name:       fmt.Sprintf("Request %d", requestCount+1),
				IsExpanded: true,
			},
			VariableExtracts: []VariableExtract{},
			Request: network.HTTPRequest{
				Dump: "GET / HTTP/1.1\r\nHost: gleip.io\r\n\r\n",
				TLS:  true,
				Host: "gleip.io",
			},
			RecalculateContentLength: true,
			GunzipResponse:           true,
			CameFrom:                 "user",
		}
	} else if stepType == "script" {
		// Count existing script steps
		scriptCount := 0
		for _, step := range gleipFlow.Steps {
			if step.StepType == "script" {
				scriptCount++
			}
		}

		newStep.ScriptStep = &ScriptStep{
			StepAttributes: gleipflow.StepAttributes{
				ID:         uuid.New().String(),
				Name:       fmt.Sprintf("Script %d", scriptCount+1),
				IsExpanded: true,
			},
			Content: "// Write your JavaScript code here\n// Examples:\n// console.log(\"Hello world\");\n// setVar(\"myVar\", \"myValue\");\n// const value = getVar(\"anotherVar\");\n",
		}
	} else if stepType == "chef" {
		// Count existing chef steps
		chefCount := 0
		for _, step := range gleipFlow.Steps {
			if step.StepType == "chef" {
				chefCount++
			}
		}

		newStep.ChefStep = &chef.ChefStep{
			StepAttributes: gleipflow.StepAttributes{
				ID:         uuid.New().String(),
				Name:       fmt.Sprintf("Chef %d", chefCount+1),
				IsExpanded: true,
			},
			InputVariable:  "",
			Actions:        []chef.ChefAction{},
			OutputVariable: "",
		}
	} else {
		// Track error
		TrackError("gleipflow", "invalid_step_type")
		return nil, fmt.Errorf("unsupported step type: %s", stepType)
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
	TrackFlowStepExecuted(gleipFlowID, stepType, true)

	// AUTOMATICALLY save the updated flow (this will update both cache and project)
	err = a.UpdateGleipFlow(*gleipFlow)
	if err != nil {
		// Track error
		TrackError("gleipflow", "save_flow_error")
		return nil, fmt.Errorf("failed to save gleipFlow: %v", err)
	}

	return &newStep, nil
}

// RemoveStepFromGleipFlow removes a step from an existing GleipFlow by index
func (a *App) RemoveStepFromGleipFlow(gleipFlowID string, stepIndex int) error {
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		// Track error
		TrackError("gleipflow", "get_flow_error")
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Validate step index
	if stepIndex < 0 || stepIndex >= len(gleipFlow.Steps) {
		return fmt.Errorf("invalid step index %d (valid range: 0-%d)", stepIndex, len(gleipFlow.Steps)-1)
	}

	// Get step type for tracking
	stepType := gleipFlow.Steps[stepIndex].StepType

	// Remove step from slice
	gleipFlow.Steps = append(gleipFlow.Steps[:stepIndex], gleipFlow.Steps[stepIndex+1:]...)

	// Track step removal
	TrackFlowStepExecuted(gleipFlowID, stepType, true)

	// AUTOMATICALLY save the updated flow (this will update both cache and project)
	err = a.UpdateGleipFlow(*gleipFlow)
	if err != nil {
		// Track error
		TrackError("gleipflow", "save_flow_error")
		return fmt.Errorf("failed to save gleipFlow: %v", err)
	}

	// Force refresh phantom requests since removing a step is a structural change
	phantomRequestState.mutex.Lock()
	// Clear the signature so next GetPhantomRequests call will refresh
	delete(phantomRequestState.lastRequestSignature, gleipFlowID)
	phantomRequestState.mutex.Unlock()

	fmt.Printf("DEBUG: Removed step %d from flow %s and cleared phantom request signature\n", stepIndex, gleipFlowID)

	return nil
}

// Domain extraction moved to telemetry.go

// DuplicateGleipFlow creates a copy of an existing GleipFlow
func (a *App) DuplicateGleipFlow(originalID string) (GleipFlow, error) {
	// Get the original flow
	originalFlow, err := a.GetGleipFlow(originalID)
	if err != nil {
		TrackError("gleipflow", "duplicate_source_not_found")
		return GleipFlow{}, fmt.Errorf("failed to get original gleipFlow: %v", err)
	}

	// Create a deep copy of the flow
	duplicatedFlow := GleipFlow{
		ID:           uuid.New().String(),
		Name:         originalFlow.Name + " (Copy)",
		Variables:    make(map[string]string),
		Steps:        make([]GleipFlowStep, len(originalFlow.Steps)),
		SortingIndex: 0, // Will be set by SaveGleipFlow
	}

	// Deep copy variables
	for k, v := range originalFlow.Variables {
		duplicatedFlow.Variables[k] = v
	}

	// Deep copy steps
	for i, step := range originalFlow.Steps {
		newStep := GleipFlowStep{
			StepType: step.StepType,
			Selected: step.Selected,
		}

		if step.RequestStep != nil {
			// Deep copy request step
			newStep.RequestStep = &RequestStep{
				StepAttributes: gleipflow.StepAttributes{
					ID:         uuid.New().String(), // New ID for the duplicated step
					Name:       step.RequestStep.StepAttributes.Name,
					IsExpanded: step.RequestStep.StepAttributes.IsExpanded,
				},
				Request:                  step.RequestStep.Request, // HTTPRequest can be copied directly
				VariableExtracts:         make([]VariableExtract, len(step.RequestStep.VariableExtracts)),
				RecalculateContentLength: step.RequestStep.RecalculateContentLength,
				GunzipResponse:           step.RequestStep.GunzipResponse,
				CameFrom:                 step.RequestStep.CameFrom, // Preserve the original cameFrom
			}

			// Deep copy variable extracts
			copy(newStep.RequestStep.VariableExtracts, step.RequestStep.VariableExtracts)

			// Deep copy fuzz settings if they exist
			if step.RequestStep.FuzzSettings != nil {
				newStep.RequestStep.FuzzSettings = &FuzzSettings{
					Delay:           step.RequestStep.FuzzSettings.Delay,
					CurrentWordlist: make([]string, len(step.RequestStep.FuzzSettings.CurrentWordlist)),
					FuzzResults:     []FuzzResult{}, // Start with empty fuzz results
				}
				copy(newStep.RequestStep.FuzzSettings.CurrentWordlist, step.RequestStep.FuzzSettings.CurrentWordlist)
			}
		}

		if step.ScriptStep != nil {
			// Deep copy script step
			newStep.ScriptStep = &ScriptStep{
				StepAttributes: gleipflow.StepAttributes{
					ID:         uuid.New().String(), // New ID for the duplicated step
					Name:       step.ScriptStep.StepAttributes.Name,
					IsExpanded: step.ScriptStep.StepAttributes.IsExpanded,
				},
				Content: step.ScriptStep.Content,
			}
		}

		if step.ChefStep != nil {
			// Deep copy chef step
			newStep.ChefStep = &chef.ChefStep{
				StepAttributes: gleipflow.StepAttributes{
					ID:         uuid.New().String(), // New ID for the duplicated step
					Name:       step.ChefStep.StepAttributes.Name,
					IsExpanded: step.ChefStep.StepAttributes.IsExpanded,
				},
				InputVariable:  step.ChefStep.InputVariable,
				OutputVariable: step.ChefStep.OutputVariable,
				Actions:        make([]chef.ChefAction, len(step.ChefStep.Actions)),
			}
			// Deep copy actions
			copy(newStep.ChefStep.Actions, step.ChefStep.Actions)
		}

		duplicatedFlow.Steps[i] = newStep
	}

	// Track duplication
	TrackFlowCreated(duplicatedFlow.ID, "duplicate")

	// AUTOMATICALLY save the duplicated flow
	err = a.UpdateGleipFlow(duplicatedFlow)
	if err != nil {
		TrackError("gleipflow", "duplicate_save_error")
		return GleipFlow{}, fmt.Errorf("failed to save duplicated gleipFlow: %v", err)
	}

	fmt.Printf("DEBUG: Successfully duplicated flow %s to %s\n", originalID, duplicatedFlow.ID)
	return duplicatedFlow, nil
}

// RenameGleipFlow renames an existing GleipFlow
func (a *App) RenameGleipFlow(gleipFlowID string, newName string) error {
	if newName == "" {
		return fmt.Errorf("new name cannot be empty")
	}

	// Get the existing flow
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		TrackError("gleipflow", "rename_flow_not_found")
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Update the name
	gleipFlow.Name = newName

	// AUTOMATICALLY save the updated flow
	err = a.UpdateGleipFlow(*gleipFlow)
	if err != nil {
		TrackError("gleipflow", "rename_save_error")
		return fmt.Errorf("failed to save renamed gleipFlow: %v", err)
	}

	fmt.Printf("DEBUG: Successfully renamed flow %s to '%s'\n", gleipFlowID, newName)
	return nil
}

// Implement RequestLike interface for PhantomRequest
func (p *PhantomRequest) GetHost() string { return p.Host }
func (p *PhantomRequest) GetTLS() bool    { return p.TLS }
func (p *PhantomRequest) GetDump() string { return p.Dump }

// ScoredTransaction represents a transaction with a relevance score
type ScoredTransaction struct {
	Transaction *network.HTTPTransaction
	Score       float64
}

// createRequestStructuralSignature creates a signature based only on structural properties of a request
// This excludes execution results to detect only meaningful changes
func createRequestStructuralSignature(lastRequest interface{}) string {
	if lastRequest == nil {
		return ""
	}

	// Try to extract structural properties from the request
	signature := map[string]interface{}{}

	// Use reflection or type assertion to extract request properties
	if reqMap, ok := lastRequest.(map[string]interface{}); ok {
		if stepAttrs, exists := reqMap["stepAttributes"]; exists {
			if attrs, ok := stepAttrs.(map[string]interface{}); ok {
				signature["stepId"] = attrs["id"]
				signature["name"] = attrs["name"]
			}
		}
		if request, exists := reqMap["request"]; exists {
			if req, ok := request.(map[string]interface{}); ok {
				signature["host"] = req["host"]
				signature["dump"] = req["dump"]
			}
		}
	}

	// Convert to JSON string for comparison
	jsonBytes, _ := json.Marshal(signature)
	return string(jsonBytes)
}

// canRefreshPhantomRequests checks if enough time has passed since last refresh
func canRefreshPhantomRequests(gleipFlowID string) bool {
	phantomRequestState.mutex.Lock()
	defer phantomRequestState.mutex.Unlock()

	lastRefresh, exists := phantomRequestState.lastRefreshTime[gleipFlowID]
	if !exists {
		return true
	}

	return time.Since(lastRefresh) >= phantomRefreshCooldown
}

// shouldRefreshPhantomRequests determines if phantom requests should be refreshed based on
// structural changes and rate limiting
func shouldRefreshPhantomRequests(gleipFlowID string, lastRequest interface{}) string {
	phantomRequestState.mutex.Lock()
	defer phantomRequestState.mutex.Unlock()

	// Skip refresh if there's an active fuzzing session for this flow
	if phantomRequestState.activeFuzzingSessions[gleipFlowID] {
		return "skip_fuzzing"
	}

	newSignature := createRequestStructuralSignature(lastRequest)

	// Check if this is the first load (no previous signature exists)
	lastSignature, signatureExists := phantomRequestState.lastRequestSignature[gleipFlowID]
	if !signatureExists {
		// First load - always allow
		phantomRequestState.lastRequestSignature[gleipFlowID] = newSignature
		phantomRequestState.lastRefreshTime[gleipFlowID] = time.Now()
		fmt.Printf("Phantom requests: First load for flow %s - allowing refresh\n", gleipFlowID)
		return "refresh"
	}

	// Check if signature changed (structural change)
	if lastSignature != newSignature {
		// Structural change detected, but check rate limit
		lastRefresh, refreshExists := phantomRequestState.lastRefreshTime[gleipFlowID]
		if !refreshExists || time.Since(lastRefresh) >= phantomRefreshCooldown {
			// Update state
			phantomRequestState.lastRequestSignature[gleipFlowID] = newSignature
			phantomRequestState.lastRefreshTime[gleipFlowID] = time.Now()
			fmt.Printf("Phantom requests: Structural change detected for flow %s - allowing refresh\n", gleipFlowID)
			return "refresh"
		} else {
			fmt.Printf("Phantom requests: Structural change detected for flow %s but rate limited (%.1fs remaining)\n", gleipFlowID, phantomRefreshCooldown.Seconds()-time.Since(lastRefresh).Seconds())
		}
	}

	return "skip_rate_limit"
}

// GetPhantomRequests generates suggested requests based on the last request in the flow
func (a *App) GetPhantomRequests(gleipFlowID string, lastRequest interface{}) ([]PhantomRequest, error) {
	// Get the flow to check for cached phantom requests
	flow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Check if we should refresh based on structural changes and rate limiting
	refreshDecision := shouldRefreshPhantomRequests(gleipFlowID, lastRequest)
	if refreshDecision == "skip_fuzzing" {
		fmt.Printf("Phantom requests: Skipping refresh for flow %s - fuzzing session active\n", gleipFlowID)
		// Return cached phantom requests if available, otherwise return nil
		if len(flow.CachedPhantomRequests) > 0 {
			fmt.Printf("Phantom requests: Returning %d cached suggestions\n", len(flow.CachedPhantomRequests))
			return flow.CachedPhantomRequests, nil
		}
		return nil, nil
	} else if refreshDecision == "skip_rate_limit" {
		fmt.Printf("Phantom requests: Skipping refresh for flow %s due to rate limiting or no structural changes\n", gleipFlowID)
		// Return cached phantom requests if available, otherwise return empty array
		if len(flow.CachedPhantomRequests) > 0 {
			fmt.Printf("Phantom requests: Returning %d cached suggestions\n", len(flow.CachedPhantomRequests))
			return flow.CachedPhantomRequests, nil
		}
		return []PhantomRequest{}, nil
	} else if refreshDecision != "refresh" {
		// Unknown decision, return cached if available
		if len(flow.CachedPhantomRequests) > 0 {
			return flow.CachedPhantomRequests, nil
		}
		return []PhantomRequest{}, nil
	}

	fmt.Printf("Phantom requests: Generating new suggestions for flow %s\n", gleipFlowID)

	// Get current request from history
	var currentTx *network.HTTPTransaction
	if flow != nil && len(flow.ExecutionResults) > 0 {
		// Get the last executed request's transaction
		for i := len(flow.ExecutionResults) - 1; i >= 0; i-- {
			if flow.ExecutionResults[i].Transaction != nil {
				currentTx = flow.ExecutionResults[i].Transaction
				break
			}
		}
	}

	// Get transaction history
	history := a.proxyServer.transactionStore.GetAll()

	if currentTx == nil || len(history) < 2 {
		// Return empty suggestions if no context or insufficient history
		return []PhantomRequest{}, nil
	}

	// Find top 3 likely next requests based on heuristics
	likelyNextRequests := a.findLikelyNextRequests(history, *currentTx)

	// Convert to PhantomRequest format
	phantomRequests := make([]PhantomRequest, 0, len(likelyNextRequests))
	for _, scoredTx := range likelyNextRequests {
		phantomRequest := PhantomRequest{
			Host: scoredTx.Transaction.Request.Host,
			TLS:  scoredTx.Transaction.Request.TLS,
			Dump: scoredTx.Transaction.Request.Dump,
		}
		phantomRequests = append(phantomRequests, phantomRequest)
	}

	// Cache the newly generated phantom requests in the flow
	flow.CachedPhantomRequests = phantomRequests
	err = a.UpdateGleipFlow(*flow)
	if err != nil {
		fmt.Printf("Warning: Failed to cache phantom requests for flow %s: %v\n", gleipFlowID, err)
		// Continue anyway, return the generated requests even if caching failed
	} else {
		fmt.Printf("Phantom requests: Cached %d suggestions for flow %s\n", len(phantomRequests), gleipFlowID)
	}

	// Track phantom request generation
	trackEvent("phantom_requests", "generated", map[string]interface{}{
		"flow_id": gleipFlowID,
		"count":   len(phantomRequests),
	})

	fmt.Printf("Phantom requests: Generated %d new suggestions for flow %s\n", len(phantomRequests), gleipFlowID)
	return phantomRequests, nil
}

// GetPhantomRequestsForced generates suggested requests bypassing rate limiting (for manual refresh)
func (a *App) GetPhantomRequestsForced(gleipFlowID string, lastRequest interface{}) ([]PhantomRequest, error) {
	// Update the refresh time to bypass rate limiting and fuzzing blocks
	phantomRequestState.mutex.Lock()
	phantomRequestState.lastRefreshTime[gleipFlowID] = time.Now()
	phantomRequestState.lastRequestSignature[gleipFlowID] = createRequestStructuralSignature(lastRequest)
	// Temporarily disable fuzzing block for forced refresh
	wasFuzzing := phantomRequestState.activeFuzzingSessions[gleipFlowID]
	phantomRequestState.activeFuzzingSessions[gleipFlowID] = false
	phantomRequestState.mutex.Unlock()

	// Restore fuzzing state at the end
	defer func() {
		phantomRequestState.mutex.Lock()
		phantomRequestState.activeFuzzingSessions[gleipFlowID] = wasFuzzing
		phantomRequestState.mutex.Unlock()
	}()

	fmt.Printf("Phantom requests: Forced refresh for flow %s\n", gleipFlowID)

	// Get the flow to understand context
	flow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Get current request from history
	var currentTx *network.HTTPTransaction
	if flow != nil && len(flow.ExecutionResults) > 0 {
		// Get the last executed request's transaction
		for i := len(flow.ExecutionResults) - 1; i >= 0; i-- {
			if flow.ExecutionResults[i].Transaction != nil {
				currentTx = flow.ExecutionResults[i].Transaction
				break
			}
		}
	}

	// Get transaction history
	history := a.proxyServer.transactionStore.GetAll()

	if currentTx == nil || len(history) < 2 {
		// Return empty suggestions if no context or insufficient history
		return []PhantomRequest{}, nil
	}

	// Find top 3 likely next requests based on heuristics
	likelyNextRequests := a.findLikelyNextRequests(history, *currentTx)

	// Convert to PhantomRequest format
	phantomRequests := make([]PhantomRequest, 0, len(likelyNextRequests))
	for _, scoredTx := range likelyNextRequests {
		phantomRequest := PhantomRequest{
			Host: scoredTx.Transaction.Request.Host,
			TLS:  scoredTx.Transaction.Request.TLS,
			Dump: scoredTx.Transaction.Request.Dump,
		}
		phantomRequests = append(phantomRequests, phantomRequest)
	}

	// Cache the newly generated phantom requests in the flow
	flow.CachedPhantomRequests = phantomRequests
	err = a.UpdateGleipFlow(*flow)
	if err != nil {
		fmt.Printf("Warning: Failed to cache phantom requests for flow %s: %v\n", gleipFlowID, err)
		// Continue anyway, return the generated requests even if caching failed
	} else {
		fmt.Printf("Phantom requests: Cached %d suggestions for forced refresh of flow %s\n", len(phantomRequests), gleipFlowID)
	}

	// Track phantom request generation
	trackEvent("phantom_requests", "forced_generated", map[string]interface{}{
		"flow_id": gleipFlowID,
		"count":   len(phantomRequests),
	})

	fmt.Printf("Phantom requests: Generated %d suggestions for forced refresh of flow %s\n", len(phantomRequests), gleipFlowID)
	return phantomRequests, nil
}

// findLikelyNextRequests analyzes history and returns top 3 most likely next requests
// with deduplication and improved heuristics
func (a *App) findLikelyNextRequests(history []network.HTTPTransaction, currentTx network.HTTPTransaction) []ScoredTransaction {
	if len(history) < 2 {
		return []ScoredTransaction{}
	}

	// Parse and normalize current transaction timestamp
	currentTime, err := time.Parse(time.RFC3339, currentTx.Timestamp)
	if err != nil {
		return []ScoredTransaction{}
	}

	candidates := make([]ScoredTransaction, 0)
	seenRequests := make(map[string]bool) // Track functionally identical requests

	// Mark the current request signature as seen to avoid suggesting identical requests
	currentRequestSignature := a.createRequestSignature(currentTx)
	seenRequests[currentRequestSignature] = true

	// Analyze each transaction in history as a potential next request
	for i, tx := range history {
		if tx.ID == currentTx.ID {
			continue // Skip the current transaction
		}

		// Create unique signature for deduplication
		requestSignature := a.createRequestSignature(tx)
		if seenRequests[requestSignature] {
			continue // Skip functionally identical requests (including duplicates of current request)
		}

		score := a.calculateHeuristicScore(history, currentTx, tx, currentTime, i)
		if score > 0 {
			candidates = append(candidates, ScoredTransaction{
				Transaction: &tx,
				Score:       score,
			})
			seenRequests[requestSignature] = true
		}
	}

	// Sort by score in descending order (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Return only top 3 results
	maxResults := 3
	if len(candidates) < maxResults {
		maxResults = len(candidates)
	}

	return candidates[:maxResults]
}

// calculateHeuristicScore calculates relevance score based on multiple improved heuristics
func (a *App) calculateHeuristicScore(history []network.HTTPTransaction, currentTx, candidateTx network.HTTPTransaction, currentTime time.Time, candidateIndex int) float64 {
	score := 0.0

	// Parse candidate timestamp
	candidateTime, err := time.Parse(time.RFC3339, candidateTx.Timestamp)
	if err != nil {
		return 0.0
	}

	// 1. TEMPORAL PROXIMITY HEURISTIC: Requests within 5 seconds get high priority
	// This captures immediate follow-up requests like redirects, AJAX calls, or form submissions
	timeDiff := candidateTime.Sub(currentTime)
	if timeDiff > 0 && timeDiff <= 5*time.Second {
		proximityScore := 25.0 * (1.0 - (timeDiff.Seconds() / 5))
		score += proximityScore
	} else if timeDiff > 0 && timeDiff <= 30*time.Second {
		// Secondary proximity window for related requests
		proximityScore := 10.0 * (1.0 - (timeDiff.Seconds() / 30))
		score += proximityScore
	}

	// 2. SEQUENTIAL PATTERN HEURISTIC: Analyze historical request sequences
	// This identifies patterns where one request consistently follows another
	sequenceScore := a.calculateSequenceScore(history, currentTx, candidateTx)
	score += sequenceScore

	// 3. SAME HOST/DOMAIN BONUS: Requests on the same domain are more likely to be related
	if currentTx.Request.Host == candidateTx.Request.Host {
		score += 15.0
	}

	// 4. SAME PROTOCOL BONUS: HTTP/HTTPS consistency indicates related flows
	if currentTx.Request.TLS == candidateTx.Request.TLS {
		score += 5.0
	}

	// 5. PATH DEPTH PROGRESSION HEURISTIC: Requests that go deeper in the same path structure
	// Example: /api/users → /api/users/123 → /api/users/123/profile
	pathScore := a.calculatePathProgressionScore(currentTx.Request.URL(), candidateTx.Request.URL())
	score += pathScore

	// 6. HTTP METHOD PATTERNS: Common request method sequences in web applications
	methodScore := a.calculateMethodPatternScore(currentTx.Request.Method(), candidateTx.Request.Method())
	score += methodScore

	// 7. SHARED COOKIES/SESSION TOKENS HEURISTIC: Requests with similar authentication
	// This identifies authenticated flows and session-based interactions
	cookieScore := a.calculateCookieSimilarityScore(currentTx, candidateTx)
	score += cookieScore

	// 8. REFERER HEADER HEURISTIC: Candidate request has Referer matching current request URL
	// This captures navigation flows and form submissions
	refererScore := a.calculateRefererScore(currentTx, candidateTx)
	score += refererScore

	// 9. LOCATION HEADER REDIRECT HEURISTIC: Current request has 3xx status with Location header
	// that matches the candidate request URL (redirect following)
	redirectScore := a.calculateRedirectScore(currentTx, candidateTx)
	score += redirectScore

	// 10. RECENCY BONUS: More recent requests get slight preference (avoid stale patterns)
	daysSinceCandidate := time.Since(candidateTime).Hours() / 24
	if daysSinceCandidate < 7 {
		score += 2.0 * (1.0 - (daysSinceCandidate / 7))
	}

	return score
}

// calculateSequenceScore analyzes how often candidateTx follows currentTx in history
func (a *App) calculateSequenceScore(history []network.HTTPTransaction, currentTx, candidateTx network.HTTPTransaction) float64 {
	sequenceCount := 0
	totalCurrentOccurrences := 0

	currentURL := currentTx.Request.URL()
	candidateURL := candidateTx.Request.URL()

	for i := 0; i < len(history)-1; i++ {
		if history[i].Request.URL() == currentURL {
			totalCurrentOccurrences++
			if i+1 < len(history) && history[i+1].Request.URL() == candidateURL {
				sequenceCount++
			}
		}
	}

	if totalCurrentOccurrences == 0 {
		return 0.0
	}

	// Return sequence probability * weight
	return (float64(sequenceCount) / float64(totalCurrentOccurrences)) * 20.0
}

// calculatePathSimilarityScore compares URL paths for similarity
func (a *App) calculatePathSimilarityScore(currentURL, candidateURL string) float64 {
	// Extract paths from URLs
	currentPath := a.extractPath(currentURL)
	candidatePath := a.extractPath(candidateURL)

	// Split paths into segments
	currentSegments := strings.Split(strings.Trim(currentPath, "/"), "/")
	candidateSegments := strings.Split(strings.Trim(candidatePath, "/"), "/")

	if len(currentSegments) == 0 || len(candidateSegments) == 0 {
		return 0.0
	}

	// Calculate segment overlap
	commonSegments := 0
	maxSegments := len(currentSegments)
	if len(candidateSegments) > maxSegments {
		maxSegments = len(candidateSegments)
	}

	minLen := len(currentSegments)
	if len(candidateSegments) < minLen {
		minLen = len(candidateSegments)
	}

	for i := 0; i < minLen; i++ {
		if currentSegments[i] == candidateSegments[i] {
			commonSegments++
		} else {
			break // Stop at first non-matching segment
		}
	}

	if maxSegments == 0 {
		return 0.0
	}

	// Return similarity score (0-8 points)
	return (float64(commonSegments) / float64(maxSegments)) * 8.0
}

// calculateMethodPatternScore assigns scores based on common HTTP method patterns
func (a *App) calculateMethodPatternScore(currentMethod, candidateMethod string) float64 {
	// Common patterns in web applications
	patterns := map[string]map[string]float64{
		"GET": {
			"POST":   3.0, // GET often followed by POST (forms)
			"PUT":    2.0, // GET then PUT (updates)
			"DELETE": 1.5, // GET then DELETE
			"GET":    1.0, // GET chains
		},
		"POST": {
			"GET":  4.0, // POST often followed by GET (redirects)
			"POST": 2.0, // POST chains
		},
		"PUT": {
			"GET": 3.0, // PUT often followed by GET (verification)
		},
		"DELETE": {
			"GET": 3.0, // DELETE often followed by GET (verification)
		},
	}

	if methodMap, exists := patterns[currentMethod]; exists {
		if score, exists := methodMap[candidateMethod]; exists {
			return score
		}
	}

	return 0.0
}

// extractPath extracts the path component from a URL
func (a *App) extractPath(urlStr string) string {
	if urlStr == "" {
		return "/"
	}

	// Remove protocol
	if strings.HasPrefix(urlStr, "http://") {
		urlStr = urlStr[7:]
	} else if strings.HasPrefix(urlStr, "https://") {
		urlStr = urlStr[8:]
	}

	// Find first slash (start of path)
	slashIndex := strings.Index(urlStr, "/")
	if slashIndex == -1 {
		return "/"
	}

	path := urlStr[slashIndex:]

	// Remove query parameters and fragments
	if queryIndex := strings.Index(path, "?"); queryIndex != -1 {
		path = path[:queryIndex]
	}
	if fragmentIndex := strings.Index(path, "#"); fragmentIndex != -1 {
		path = path[:fragmentIndex]
	}

	return path
}

// createRequestSignature creates a unique signature for request deduplication
// Combines method, path, query parameters, and body to identify functionally identical requests
func (a *App) createRequestSignature(tx network.HTTPTransaction) string {
	method := tx.Request.Method()
	url := tx.Request.URL()
	body := string(tx.Request.Body())

	// Normalize the signature to catch functionally equivalent requests
	return fmt.Sprintf("%s|%s|%s", method, url, body)
}

// calculatePathProgressionScore analyzes if the candidate request goes deeper in the same path structure
// Higher scores for requests that extend the current path (e.g., /api/users → /api/users/123)
func (a *App) calculatePathProgressionScore(currentURL, candidateURL string) float64 {
	currentPath := a.extractPath(currentURL)
	candidatePath := a.extractPath(candidateURL)

	// Split paths into segments
	currentSegments := strings.Split(strings.Trim(currentPath, "/"), "/")
	candidateSegments := strings.Split(strings.Trim(candidatePath, "/"), "/")

	// Filter out empty segments
	currentSegments = a.filterEmptyStrings(currentSegments)
	candidateSegments = a.filterEmptyStrings(candidateSegments)

	if len(currentSegments) == 0 || len(candidateSegments) == 0 {
		return 0.0
	}

	// Check if candidate path extends current path (deeper navigation)
	if len(candidateSegments) > len(currentSegments) {
		// Check if current path is a prefix of candidate path
		isPrefix := true
		for i := 0; i < len(currentSegments); i++ {
			if currentSegments[i] != candidateSegments[i] {
				isPrefix = false
				break
			}
		}
		if isPrefix {
			// Score based on how much deeper it goes (up to 12 points)
			depthIncrease := len(candidateSegments) - len(currentSegments)
			return 12.0 / float64(depthIncrease) // More points for immediate depth increase
		}
	}

	// Fallback to similarity scoring for related paths
	return a.calculatePathSimilarityScore(currentURL, candidateURL)
}

// calculateCookieSimilarityScore analyzes shared cookies and session tokens between requests
// Higher scores for requests with similar authentication context
func (a *App) calculateCookieSimilarityScore(currentTx, candidateTx network.HTTPTransaction) float64 {
	currentHeaders := currentTx.Request.Headers()
	candidateHeaders := candidateTx.Request.Headers()

	currentCookies := a.extractCookies(currentHeaders["Cookie"])
	candidateCookies := a.extractCookies(candidateHeaders["Cookie"])

	if len(currentCookies) == 0 || len(candidateCookies) == 0 {
		return 0.0
	}

	// Count shared cookies, with extra weight for session/auth tokens
	sharedCount := 0
	authTokenBonus := 0.0

	for cookieName, currentValue := range currentCookies {
		if candidateValue, exists := candidateCookies[cookieName]; exists && candidateValue == currentValue {
			sharedCount++

			// Extra points for authentication-related cookies
			lowerName := strings.ToLower(cookieName)
			if strings.Contains(lowerName, "session") || strings.Contains(lowerName, "auth") ||
				strings.Contains(lowerName, "token") || strings.Contains(lowerName, "csrf") {
				authTokenBonus += 3.0
			}
		}
	}

	totalCookies := len(currentCookies)
	if len(candidateCookies) > totalCookies {
		totalCookies = len(candidateCookies)
	}

	if totalCookies == 0 {
		return 0.0
	}

	// Base score from shared cookies + auth token bonus (up to 10 points + bonus)
	baseScore := (float64(sharedCount) / float64(totalCookies)) * 10.0
	return baseScore + authTokenBonus
}

// calculateRefererScore checks if candidate request has Referer header matching current request URL
// This identifies navigation flows where one request leads to another
func (a *App) calculateRefererScore(currentTx, candidateTx network.HTTPTransaction) float64 {
	candidateHeaders := candidateTx.Request.Headers()
	referer := candidateHeaders["Referer"]

	if referer == "" {
		referer = candidateHeaders["referer"] // Try lowercase
	}

	if referer == "" {
		return 0.0
	}

	currentURL := currentTx.Request.URL()

	// Exact match gets full points
	if referer == currentURL {
		return 15.0
	}

	// Partial match for different query parameters but same base URL
	currentBase := strings.Split(currentURL, "?")[0]
	refererBase := strings.Split(referer, "?")[0]

	if currentBase == refererBase {
		return 8.0
	}

	return 0.0
}

// calculateRedirectScore checks if current request has 3xx status with Location header
// that matches the candidate request URL (redirect following pattern)
func (a *App) calculateRedirectScore(currentTx, candidateTx network.HTTPTransaction) float64 {
	// Check if current request has a response with redirect status
	if currentTx.Response == nil {
		return 0.0
	}

	statusCode := currentTx.Response.StatusCode()
	if statusCode < 300 || statusCode >= 400 {
		return 0.0 // Not a redirect response
	}

	// Extract Location header from current response
	locationHeader := a.extractLocationHeader(currentTx.Response.Dump)
	if locationHeader == "" {
		return 0.0
	}

	candidateURL := candidateTx.Request.URL()

	// Exact match gets high score (redirect following)
	if locationHeader == candidateURL {
		return 20.0
	}

	// Relative URL handling - convert to absolute for comparison
	if strings.HasPrefix(locationHeader, "/") {
		// Construct full URL
		scheme := "http"
		if currentTx.Request.TLS {
			scheme = "https"
		}
		fullLocation := fmt.Sprintf("%s://%s%s", scheme, currentTx.Request.Host, locationHeader)
		if fullLocation == candidateURL {
			return 20.0
		}
	}

	return 0.0
}

// Helper methods

// filterEmptyStrings removes empty strings from a slice
func (a *App) filterEmptyStrings(strs []string) []string {
	result := make([]string, 0, len(strs))
	for _, s := range strs {
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// extractCookies parses cookie string into a map
func (a *App) extractCookies(cookieHeader string) map[string]string {
	cookies := make(map[string]string)
	if cookieHeader == "" {
		return cookies
	}

	// Split by semicolon and parse each cookie
	parts := strings.Split(cookieHeader, ";")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "=") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) == 2 {
				cookies[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}

	return cookies
}

// extractLocationHeader extracts Location header from HTTP response dump
func (a *App) extractLocationHeader(responseDump string) string {
	lines := strings.Split(responseDump, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "location:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// AddPhantomRequestToGleipFlow adds a phantom request as a new request step at the end of the flow
func (a *App) AddPhantomRequestToGleipFlow(gleipFlowID string, phantomRequest PhantomRequest) error {
	// Get the existing flow
	gleipFlow, err := a.GetGleipFlow(gleipFlowID)
	if err != nil {
		TrackError("gleipflow", "phantom_request_flow_not_found")
		return fmt.Errorf("failed to get gleipFlow: %v", err)
	}

	// Count existing request steps for naming
	requestCount := 0
	for _, step := range gleipFlow.Steps {
		if step.StepType == "request" {
			requestCount++
		}
	}

	// Create a new request step from the phantom request
	newStep := GleipFlowStep{
		StepType: "request",
		Selected: true,
		RequestStep: &RequestStep{
			StepAttributes: gleipflow.StepAttributes{
				ID:         uuid.New().String(),
				Name:       fmt.Sprintf("Request %d", requestCount+1),
				IsExpanded: true,
			},
			Request: network.HTTPRequest{
				Host: phantomRequest.Host,
				TLS:  phantomRequest.TLS,
				Dump: phantomRequest.Dump,
			},
			VariableExtracts:         []VariableExtract{},
			RecalculateContentLength: true,
			GunzipResponse:           true,
			CameFrom:                 "history",
		},
	}

	// Add the step to the end of the flow
	gleipFlow.Steps = append(gleipFlow.Steps, newStep)

	// Track phantom request addition
	trackEvent("phantom_requests", "added", map[string]interface{}{
		"flow_id": gleipFlowID,
		"host":    phantomRequest.Host,
		"tls":     phantomRequest.TLS,
	})

	// Save the updated flow
	err = a.UpdateGleipFlow(*gleipFlow)
	if err != nil {
		TrackError("gleipflow", "phantom_request_save_error")
		return fmt.Errorf("failed to save gleipFlow with phantom request: %v", err)
	}

	fmt.Printf("DEBUG: Successfully added phantom request to flow %s\n", gleipFlowID)
	return nil
}
