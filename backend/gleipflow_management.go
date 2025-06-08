package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/network"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Global cancellation variable for the fuzzing process
var fuzzCancellation = make(chan struct{})
var fuzzMutex sync.Mutex

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

	// Find the request step
	var requestStep *RequestStep
	for i, step := range gleipFlow.Steps {
		if step.StepType == "request" && step.RequestStep != nil && step.RequestStep.ID == stepID {
			requestStep = step.RequestStep

			// Make sure only this step is selected for execution
			for j := range gleipFlow.Steps {
				gleipFlow.Steps[j].Selected = (j == i)
			}
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

	// Reset the cancellation channel
	fuzzCancellation = make(chan struct{})

	// Execute the flow (which will only execute the selected step)
	_, err = a.gleipFlowExecutor.ExecuteGleipFlow(gleipFlow)
	if err != nil {
		// Track error
		TrackError("fuzzing", "execution_error")
		return fmt.Errorf("failed to execute fuzz: %v", err)
	}

	// Save the updated flow with fuzz results
	_, err = a.SaveGleipFlow(*gleipFlow)
	if err != nil {
		// Track error
		TrackError("fuzzing", "save_results_error")
		return fmt.Errorf("failed to save fuzz results: %v", err)
	}

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
				ID:   uuid.New().String(),
				Name: fmt.Sprintf("Request %d", requestCount+1),
				Request: network.HTTPRequest{
					Host: apiRequest.Host,
					Dump: rawRequest,
					TLS:  strings.HasPrefix(strings.ToLower(apiRequest.URL), "https://") || strings.HasPrefix(strings.ToLower(trackingURL), "https://"), // Determine TLS from URL
				},
				VariableExtracts:         []VariableExtract{},
				RecalculateContentLength: true,
				GunzipResponse:           true,
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
				ID:                       uuid.New().String(),
				Name:                     fmt.Sprintf("Request %d", requestCount+1),
				Request:                  transaction.Request,
				VariableExtracts:         []VariableExtract{},
				RecalculateContentLength: true,
				GunzipResponse:           true,
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
			ID:               uuid.New().String(),
			Name:             fmt.Sprintf("Request %d", requestCount+1),
			VariableExtracts: []VariableExtract{},
			Request: network.HTTPRequest{
				Dump: "GET / HTTP/1.1\r\nHost: gleip.io\r\n\r\n",
				TLS:  true,
				Host: "gleip.io",
			},
			RecalculateContentLength: true,
			GunzipResponse:           true,
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
			ID:      uuid.New().String(),
			Name:    fmt.Sprintf("Script %d", scriptCount+1),
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
			ID:             uuid.New().String(),
			Name:           fmt.Sprintf("Chef %d", chefCount+1),
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
				ID:                       uuid.New().String(), // New ID for the duplicated step
				Name:                     step.RequestStep.Name,
				Request:                  step.RequestStep.Request, // HTTPRequest can be copied directly
				VariableExtracts:         make([]VariableExtract, len(step.RequestStep.VariableExtracts)),
				RecalculateContentLength: step.RequestStep.RecalculateContentLength,
				GunzipResponse:           step.RequestStep.GunzipResponse,
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
				ID:      uuid.New().String(), // New ID for the duplicated step
				Name:    step.ScriptStep.Name,
				Content: step.ScriptStep.Content,
			}
		}

		if step.ChefStep != nil {
			// Deep copy chef step
			newStep.ChefStep = &chef.ChefStep{
				ID:             uuid.New().String(), // New ID for the duplicated step
				Name:           step.ChefStep.Name,
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
