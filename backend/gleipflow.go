package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/network"
	"Gleip/backend/network/http_utils"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GleipFlowExecutor handles the execution of request gleipFlows
// Now follows Single Responsibility Principle - only responsible for orchestrating flow execution
type GleipFlowExecutor struct {
	app                  *App
	requestSender        network.RequestSender
	variableProcessor    VariableProcessor
	variableExtractor    VariableExtractor
	scriptExecutor       ScriptExecutor
	eventEmitter         TransactionEventEmitter
	responseDecompressor network.ResponseDecompressor
}

// NewGleipFlowExecutor creates a new GleipFlowExecutor with dependencies injected
func NewGleipFlowExecutor(app *App) *GleipFlowExecutor {
	// Create HTTP client with timeouts
	httpClient := &http.Client{
		Transport: network.CreateHTTPTransport(),
		Timeout:   60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Create dependencies
	responseFormatter := network.NewResponseFormatter()
	responseDecompressor := network.NewResponseDecompressor()
	requestSender := network.NewRequestSender(httpClient, responseDecompressor, responseFormatter)

	return &GleipFlowExecutor{
		app:                  app,
		requestSender:        requestSender,
		variableProcessor:    NewVariableProcessor(),
		variableExtractor:    NewVariableExtractor(),
		scriptExecutor:       NewDefaultScriptExecutor(),
		eventEmitter:         NewDefaultEventEmitter(app),
		responseDecompressor: responseDecompressor,
	}
}

// ExecuteGleipFlow executes a request gleipFlow and returns the results
func (e *GleipFlowExecutor) ExecuteGleipFlow(gleipFlow *GleipFlow) ([]ExecutionResult, error) {
	ctx := NewExecutionContext()
	ctx.Variables = gleipFlow.Variables

	// Count selected steps
	selectedSteps := 0
	for _, step := range gleipFlow.Steps {
		if step.Selected {
			selectedSteps++
		}
	}

	// Track flow execution
	TrackFlowExecuted(gleipFlow.ID, selectedSteps, true)

	results := make([]ExecutionResult, 0, len(gleipFlow.Steps))

	// Execute each step in sequence
	for i, step := range gleipFlow.Steps {
		// Only execute selected steps
		if !step.Selected {
			continue
		}

		var result ExecutionResult
		startTime := time.Now()

		// Use strategy pattern for different step types
		switch step.StepType {
		case "request":
			if step.RequestStep != nil {
				result = e.executeRequestStep(step.RequestStep, ctx)
			} else {
				result = createErrorResult("", "Unknown", "request", "Request step is nil")
			}
		case "script":
			if step.ScriptStep != nil {
				result = e.executeScriptStep(step.ScriptStep, ctx)
			} else {
				result = createErrorResult("", "Unknown", "script", "Script step is nil")
			}
		case "chef":
			if step.ChefStep != nil {
				result = e.ExecuteChefStep(step.ChefStep, ctx)
			} else {
				result = createErrorResult("", "Unknown", "chef", "Chef step is nil")
			}
		default:
			result = createErrorResult("", "Unknown", "unknown", fmt.Sprintf("Unsupported step type: %s", step.StepType))
		}

		// Set step ID and name if not already set
		if result.StepID == "" {
			result.StepID = getStepID(step)
			result.StepName = getStepName(step)
		}

		// Calculate execution time
		result.ExecutionTime = time.Since(startTime).Milliseconds()

		// Add result to results
		results = append(results, result)

		// Add the result to the execution context
		ctx.Results = append(ctx.Results, result)

		// Merge variables from step result back into execution context
		if result.Variables != nil {
			for varName, varValue := range result.Variables {
				ctx.Variables[varName] = varValue
			}
		}

		// Update action previews for subsequent chef steps that use the newly created variables
		if len(result.Variables) > 0 {
			// Convert result variables to map[string]bool
			updatedVarNames := make(map[string]bool)
			for varName := range result.Variables {
				updatedVarNames[varName] = true
			}

			// Update action previews for chef steps that use the newly created variables
			if e.app != nil {
				e.app.updateChefStepActionPreviews(gleipFlow, updatedVarNames)
			}
		}

		// Emit the current results to the frontend
		if e.eventEmitter != nil {
			e.eventEmitter.EmitStepExecuted(gleipFlow.ID, i, results)
		}

		// Track errors
		if !result.Success {
			TrackFlowStepExecuted(gleipFlow.ID, result.StepType, false)
			TrackError("flow_step", result.ErrorMessage)
		} else if i == len(gleipFlow.Steps)-1 || !gleipFlow.Steps[i+1].Selected {
			TrackFlowStepExecuted(gleipFlow.ID, result.StepType, true)
		}
	}

	return results, nil
}

// executeRequestStep executes a single request step
func (e *GleipFlowExecutor) executeRequestStep(step *RequestStep, ctx *ExecutionContext) ExecutionResult {
	result := ExecutionResult{
		StepID:   step.StepAttributes.ID,
		StepName: step.StepAttributes.Name,
		StepType: "request",
		Success:  true,
	}

	// Check if this is a fuzz request
	if step.FuzzSettings != nil && len(step.FuzzSettings.CurrentWordlist) > 0 {
		return e.executeFuzzStep(step, ctx)
	}

	// Process variables using the variable processor
	processedMethod := e.variableProcessor.ProcessVariables(step.Request.Method(), ctx.Variables)
	processedURL := e.variableProcessor.ProcessVariables(step.Request.URL(), ctx.Variables)
	processedHost := e.variableProcessor.ProcessVariables(step.Request.Host, ctx.Variables)

	var transaction *network.HTTPTransaction
	var err error
	var actualRawRequest string

	if step.Request.Dump != "" {
		actualRawRequest, transaction, err = e.executeRawRequest(step, ctx)
	} else {
		actualRawRequest, transaction, err = e.executeBuilderRequest(step, ctx, processedMethod, processedURL, processedHost)
	}

	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Failed to send request: %v", err)
		TrackRequestSent(processedMethod, 0, false)
		TrackError("http_request", result.ErrorMessage)
		return result
	}

	result.Transaction = transaction
	result.ActualRawRequest = actualRawRequest

	// Add transaction to history
	if e.app != nil && e.app.proxyServer != nil && transaction != nil {
		transactionCopy := *transaction
		go e.app.proxyServer.AddTransactionToHistory(transactionCopy)
	}

	// Track successful request
	if transaction != nil && transaction.Response != nil {
		TrackRequestSent(transaction.Request.Method(), transaction.Response.StatusCode(), false)
	}

	// Extract variables from response
	result.Variables = e.extractVariablesFromResponse(step.VariableExtracts, transaction, ctx)

	return result
}

// executeRawRequest handles raw request execution
func (e *GleipFlowExecutor) executeRawRequest(step *RequestStep, ctx *ExecutionContext) (string, *network.HTTPTransaction, error) {
	options := RequestExecutionOptions{
		VariableProcessor: func(rawRequest string) string {
			return e.variableProcessor.ProcessVariables(rawRequest, ctx.Variables)
		},
		RecalculateLength: step.RecalculateContentLength,
		GunzipResponse:    step.GunzipResponse,
	}

	result := e.executeRequestWithOptions(step, options)
	return result.ActualRawRequest, result.Transaction, result.Error
}

// executeBuilderRequest handles builder mode request execution
func (e *GleipFlowExecutor) executeBuilderRequest(step *RequestStep, ctx *ExecutionContext, processedMethod, processedURL, processedHost string) (string, *network.HTTPTransaction, error) {
	bodyWithPlaceholders := string(step.Request.Body())
	bodyForSending := e.variableProcessor.ProcessVariables(string(step.Request.Body()), ctx.Variables)

	processedHeadersForSending := make(map[string]string)
	for k, v := range step.Request.Headers() {
		processedHeaderKey := e.variableProcessor.ProcessVariables(k, ctx.Variables)
		processedHeadersForSending[processedHeaderKey] = e.variableProcessor.ProcessVariables(v, ctx.Variables)
	}

	// Construct actualRawRequest for UI
	var actualRawRequestBuilder strings.Builder
	actualRawRequestBuilder.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", step.Request.Method(), step.Request.URL()))

	finalHeadersForUIMap := make(map[string]string)
	for k, v := range step.Request.Headers() {
		finalHeadersForUIMap[k] = v
	}

	if step.RecalculateContentLength {
		newContentLength := len(bodyForSending)
		processedHeadersForSending["Content-Length"] = fmt.Sprintf("%d", newContentLength)
		finalHeadersForUIMap["Content-Length"] = fmt.Sprintf("%d", newContentLength)
	}

	for k, v := range finalHeadersForUIMap {
		actualRawRequestBuilder.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	actualRawRequestBuilder.WriteString("\r\n")
	if bodyWithPlaceholders != "" {
		actualRawRequestBuilder.WriteString(bodyWithPlaceholders)
	}

	actualRawRequest := actualRawRequestBuilder.String()
	transaction, err := e.requestSender.SendRequest(processedMethod, processedURL, processedHost, bodyForSending, processedHeadersForSending, step.GunzipResponse, step.Request.TLS)

	return actualRawRequest, transaction, err
}

// executeFuzzStep executes a fuzz request step
func (e *GleipFlowExecutor) executeFuzzStep(step *RequestStep, ctx *ExecutionContext) ExecutionResult {
	result := ExecutionResult{
		StepID:   step.StepAttributes.ID,
		StepName: step.StepAttributes.Name,
		StepType: "request",
		Success:  true,
	}

	TrackFuzzingStarted("", step.StepAttributes.ID, len(step.FuzzSettings.CurrentWordlist))

	if step.FuzzSettings.FuzzResults == nil {
		step.FuzzSettings.FuzzResults = []FuzzResult{}
	}

	fmt.Printf("Starting fuzzing with %d words and %.2fs delay\n", len(step.FuzzSettings.CurrentWordlist), step.FuzzSettings.Delay)

	// Create cancellation mechanism
	done := make(chan struct{})
	cancelled := false

	go func() {
		select {
		case <-fuzzCancellation:
			cancelled = true
			close(done)
		case <-done:
		}
	}()

	// Execute fuzzing
	for _, word := range step.FuzzSettings.CurrentWordlist {
		select {
		case <-done:
			fmt.Printf("Fuzzing cancelled after %d results\n", len(step.FuzzSettings.FuzzResults))
			result.Success = true
			result.Transaction = createSampleTransaction(step, step.FuzzSettings.FuzzResults)
			TrackFuzzingCompleted("", step.StepAttributes.ID, len(step.FuzzSettings.FuzzResults), true)
			return result
		default:
		}

		// Execute single fuzz request
		fuzzResult := e.executeSingleFuzzRequest(step, ctx, word)
		step.FuzzSettings.FuzzResults = append(step.FuzzSettings.FuzzResults, fuzzResult)

		fmt.Printf("Fuzz result for '%s': status=%d, size=%d, time=%dms\n",
			word, fuzzResult.StatusCode, fuzzResult.Size, fuzzResult.Time)

		// Emit real-time update (but with isFuzzing flag so frontend can handle appropriately)
		if e.eventEmitter != nil {
			e.eventEmitter.EmitFuzzUpdate(step.StepAttributes.ID, step.FuzzSettings.FuzzResults)
		}

		// Sleep between requests
		if step.FuzzSettings.Delay > 0 && !cancelled {
			e.sleepWithCancellation(time.Duration(step.FuzzSettings.Delay*float64(time.Second)), done)
		}
	}

	if !cancelled {
		close(done)
	}

	fmt.Printf("Fuzzing completed with %d results\n", len(step.FuzzSettings.FuzzResults))

	result.Transaction = createSampleTransaction(step, step.FuzzSettings.FuzzResults)
	result.ActualRawRequest = step.Request.Dump

	TrackFuzzingCompleted("", step.StepAttributes.ID, len(step.FuzzSettings.FuzzResults), cancelled)

	return result
}

// executeSingleFuzzRequest executes a single fuzz request
func (e *GleipFlowExecutor) executeSingleFuzzRequest(step *RequestStep, ctx *ExecutionContext, fuzzWord string) FuzzResult {
	timeout := 10 * time.Second

	options := RequestExecutionOptions{
		VariableProcessor: func(rawRequest string) string {
			// First process all normal variables
			processedRequest := e.variableProcessor.ProcessVariables(rawRequest, ctx.Variables)
			// Then replace the fuzz placeholder
			return strings.ReplaceAll(processedRequest, "{{fuzz}}", fuzzWord)
		},
		Timeout:           &timeout,
		RecalculateLength: step.RecalculateContentLength,
		GunzipResponse:    step.GunzipResponse,
		AdditionalMetadata: map[string]interface{}{
			"fuzzWord": fuzzWord,
		},
	}

	result := e.executeRequestWithOptions(step, options)

	var fuzzResult FuzzResult
	fuzzResult.Word = fuzzWord
	fuzzResult.Request = result.ProcessedRawRequest // Use processed request to show actual fuzzed content
	fuzzResult.Time = result.ExecutionTime.Milliseconds()

	if result.Error != nil {
		fmt.Printf("Fuzzing error with word '%s': %v\n", fuzzWord, result.Error)
	} else if result.Transaction != nil && result.Transaction.Response != nil {
		fuzzResult.Response = result.Transaction.Response.Dump
		fuzzResult.StatusCode = result.Transaction.Response.StatusCode()
		fuzzResult.Size = len(result.Transaction.Response.Dump)
	}

	return fuzzResult
}

// sleepWithCancellation sleeps for the specified duration or until cancelled
func (e *GleipFlowExecutor) sleepWithCancellation(duration time.Duration, done <-chan struct{}) {
	sleepUntil := time.Now().Add(duration)
	for time.Now().Before(sleepUntil) {
		select {
		case <-done:
			return
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
}

// executeScriptStep executes a script step
func (e *GleipFlowExecutor) executeScriptStep(step *ScriptStep, ctx *ExecutionContext) ExecutionResult {
	result := ExecutionResult{
		StepID:   step.StepAttributes.ID,
		StepName: step.StepAttributes.Name,
		StepType: "script",
		Success:  true,
	}

	extractedVars, err := e.scriptExecutor.Execute(step.Content, ctx)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Script execution failed: %v", err)
		TrackError("script", result.ErrorMessage)
		return result
	}

	result.Variables = extractedVars
	return result
}

// ExecuteChefStep executes a chef step
func (e *GleipFlowExecutor) ExecuteChefStep(step *chef.ChefStep, ctx *ExecutionContext) ExecutionResult {
	result := ExecutionResult{
		StepID:   step.StepAttributes.ID,
		StepName: step.StepAttributes.Name,
		StepType: "chef",
		Success:  true,
	}

	// Skip execution if no input variable is specified
	if step.InputVariable == "" {
		// No input variable specified - skip execution entirely
		return result
	}

	// Get the input value from variables
	inputValue, exists := ctx.Variables[step.InputVariable]
	if !exists {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Input variable '%s' not found", step.InputVariable)
		TrackError("chef", result.ErrorMessage)
		return result
	}

	// Execute the chef step
	outputValue, err := chef.ExecuteChefStep(step, inputValue)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("Chef step execution failed: %v", err)
		TrackError("chef", result.ErrorMessage)
		TrackChefStepExecuted("", step.StepAttributes.ID, len(step.Actions), false)
		return result
	}

	// Set the output variable
	if step.OutputVariable != "" {
		extractedVars := make(map[string]string)
		extractedVars[step.OutputVariable] = outputValue
		ctx.SetVariable(step.OutputVariable, outputValue, "chef step execution")
		result.Variables = extractedVars
	}

	// Track successful chef step execution
	TrackChefStepExecuted("", step.StepAttributes.ID, len(step.Actions), true)

	return result
}

// extractVariablesFromResponse extracts variables from the response
func (e *GleipFlowExecutor) extractVariablesFromResponse(extracts []VariableExtract, transaction *network.HTTPTransaction, ctx *ExecutionContext) map[string]string {
	extractedVars := make(map[string]string)

	if transaction == nil || transaction.Response == nil {
		return extractedVars
	}

	for _, extract := range extracts {
		// Process variables in the selector
		processedSelector := e.variableProcessor.ProcessVariables(extract.Selector, ctx.Variables)

		processedExtract := VariableExtract{
			Name:     extract.Name,
			Source:   extract.Source,
			Selector: processedSelector,
		}

		value, err := e.variableExtractor.Extract(processedExtract, transaction)
		if err != nil {
			fmt.Printf("Failed to extract variable %s: %v\n", extract.Name, err)
			continue
		}

		extractedVars[extract.Name] = value
		ctx.SetVariable(extract.Name, value, fmt.Sprintf("extraction from %s response", extract.Source))
	}

	return extractedVars
}

// Helper functions

func createErrorResult(stepID, stepName, stepType, errorMessage string) ExecutionResult {
	return ExecutionResult{
		StepID:       stepID,
		StepName:     stepName,
		StepType:     stepType,
		Success:      false,
		ErrorMessage: errorMessage,
	}
}

func getStepID(step GleipFlowStep) string {
	if step.StepType == "request" && step.RequestStep != nil {
		return step.RequestStep.StepAttributes.ID
	} else if step.StepType == "script" && step.ScriptStep != nil {
		return step.ScriptStep.StepAttributes.ID
	} else if step.StepType == "chef" && step.ChefStep != nil {
		return step.ChefStep.StepAttributes.ID
	}
	return ""
}

func getStepName(step GleipFlowStep) string {
	if step.StepType == "request" && step.RequestStep != nil {
		return step.RequestStep.StepAttributes.Name
	} else if step.StepType == "script" && step.ScriptStep != nil {
		return step.ScriptStep.StepAttributes.Name
	} else if step.StepType == "chef" && step.ChefStep != nil {
		return step.ChefStep.StepAttributes.Name
	}
	return "Unknown"
}

func createSampleTransaction(step *RequestStep, fuzzResults []FuzzResult) *network.HTTPTransaction {
	if len(fuzzResults) == 0 {
		return nil
	}

	sampleResult := fuzzResults[0]

	return &network.HTTPTransaction{
		ID: uuid.New().String(),
		Request: network.HTTPRequest{
			TLS:  true,
			Dump: "Sample request for fuzz results",
			Host: "gleip.io",
		},
		Response: &network.HTTPResponse{
			Dump: sampleResult.Response,
		},
		Timestamp: time.Now().Format(time.RFC3339),
		CameFrom:  "gleipflow",
	}
}

// DefaultScriptExecutor implements the ScriptExecutor interface
type DefaultScriptExecutor struct{}

// NewDefaultScriptExecutor creates a new script executor
func NewDefaultScriptExecutor() ScriptExecutor {
	return &DefaultScriptExecutor{}
}

// Execute executes a script and returns extracted variables
func (e *DefaultScriptExecutor) Execute(script string, context *ExecutionContext) (map[string]string, error) {
	// Create a new JavaScript runtime
	vm := goja.New()

	// Set up console.log
	console := make(map[string]interface{})
	console["log"] = func(call goja.FunctionCall) goja.Value {
		args := make([]interface{}, len(call.Arguments))
		for i, arg := range call.Arguments {
			args[i] = arg.Export()
		}
		fmt.Printf("Script log: %v\n", args)
		return goja.Undefined()
	}
	vm.Set("console", console)

	// Set up helper functions
	e.setupHelperFunctions(vm, context)

	// Create variables map
	variables := make(map[string]interface{})
	for k, v := range context.Variables {
		variables[k] = v
	}
	vm.Set("vars", variables)

	// Create results array
	results := e.createResultsArray(context)
	vm.Set("previousResults", results)

	// Run the script
	_, err := vm.RunString(script)
	if err != nil {
		return nil, err
	}

	// Get updated variables
	extractedVars := make(map[string]string)
	if jsVars, ok := vm.Get("vars").Export().(map[string]interface{}); ok {
		for k, v := range jsVars {
			if strVal, ok := v.(string); ok && context.Variables[k] != strVal {
				extractedVars[k] = strVal
				context.Variables[k] = strVal
			}
		}
	}

	return extractedVars, nil
}

func (e *DefaultScriptExecutor) setupHelperFunctions(vm *goja.Runtime, context *ExecutionContext) {
	vm.Set("setVar", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) != 2 {
			return vm.ToValue(fmt.Errorf("setVar requires 2 arguments: name and value"))
		}

		name := call.Arguments[0].String()
		value := call.Arguments[1].String()

		// Prevent empty variable names
		if strings.TrimSpace(name) == "" {
			return vm.ToValue(fmt.Errorf("variable name cannot be empty"))
		}

		context.SetVariable(name, value, "script execution")
		return goja.Undefined()
	})

	vm.Set("getVar", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) != 1 {
			return vm.ToValue(fmt.Errorf("getVar requires 1 argument: name"))
		}

		name := call.Arguments[0].String()
		value, exists := context.Variables[name]
		if !exists {
			return goja.Null()
		}
		return vm.ToValue(value)
	})

	vm.Set("debugVars", func(call goja.FunctionCall) goja.Value {
		fmt.Printf("\n=== CURRENT VARIABLES IN SCRIPT CONTEXT ===\n")
		if len(context.Variables) == 0 {
			fmt.Printf("  WARNING: No variables available\n")
		} else {
			for k, v := range context.Variables {
				fmt.Printf("  %s = %s\n", k, v)
			}
		}
		fmt.Printf("=========================================\n\n")
		return goja.Undefined()
	})

	// Add HTTP utility
	httpUtil := make(map[string]interface{})
	httpUtil["parseJSON"] = func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) != 1 {
			return vm.ToValue(fmt.Errorf("parseJSON requires 1 argument: jsonString"))
		}

		jsonStr := call.Arguments[0].String()
		var parsed interface{}
		err := json.Unmarshal([]byte(jsonStr), &parsed)
		if err != nil {
			return vm.ToValue(fmt.Sprintf("Error parsing JSON: %v", err))
		}
		return vm.ToValue(parsed)
	}
	vm.Set("http", httpUtil)
}

func (e *DefaultScriptExecutor) createResultsArray(context *ExecutionContext) []map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(context.Results))
	for _, r := range context.Results {
		resultMap := map[string]interface{}{
			"stepId":   r.StepID,
			"stepName": r.StepName,
			"stepType": r.StepType,
			"success":  r.Success,
		}

		if r.ErrorMessage != "" {
			resultMap["errorMessage"] = r.ErrorMessage
		}

		if r.Transaction != nil {
			transactionMap := map[string]interface{}{
				"id":        r.Transaction.ID,
				"timestamp": r.Transaction.Timestamp,
				"request": map[string]interface{}{
					"method": r.Transaction.Request.Method,
					"url":    r.Transaction.Request.URL,
				},
			}

			if r.Transaction.Response != nil {
				transactionMap["response"] = map[string]interface{}{
					"status":     r.Transaction.Response.Status,
					"statusCode": r.Transaction.Response.StatusCode,
				}
			}

			resultMap["transaction"] = transactionMap
		}

		results = append(results, resultMap)
	}
	return results
}

// DefaultEventEmitter implements the TransactionEventEmitter interface
type DefaultEventEmitter struct {
	app *App
}

// NewDefaultEventEmitter creates a new event emitter
func NewDefaultEventEmitter(app *App) TransactionEventEmitter {
	return &DefaultEventEmitter{app: app}
}

func (e *DefaultEventEmitter) EmitNewTransaction(transaction network.HTTPTransaction) {
	if e.app != nil && e.app.ctx != nil {
		// Add transaction to project's RequestHistory for persistence
		e.app.projectMutex.Lock()
		if e.app.currentProject != nil {
			// Add new transaction to project history
			if e.app.currentProject.RequestHistory == nil {
				e.app.currentProject.RequestHistory = []*network.HTTPTransaction{}
			}
			transactionCopy := transaction // Make a copy
			e.app.currentProject.RequestHistory = append(e.app.currentProject.RequestHistory, &transactionCopy)
		}
		e.app.projectMutex.Unlock()

		// Request autosave since we added a new request to history
		e.app.requestAutoSaveWithComponent("request_history")

		// Emit the frontend event
		runtime.EventsEmit(e.app.ctx, "new_transaction")
	}
}

func (e *DefaultEventEmitter) EmitTransactionUpdate(transaction network.HTTPTransaction) {
	if e.app != nil && e.app.ctx != nil {
		// Update transaction in project's RequestHistory for persistence
		e.app.projectMutex.Lock()
		if e.app.currentProject != nil && e.app.currentProject.RequestHistory != nil {
			// Find and update the corresponding transaction in project history
			for i, tx := range e.app.currentProject.RequestHistory {
				if tx != nil && tx.ID == transaction.ID {
					transactionCopy := transaction // Make a copy
					e.app.currentProject.RequestHistory[i] = &transactionCopy
					break
				}
			}
		}
		e.app.projectMutex.Unlock()

		// Request autosave since we updated a request in history
		e.app.requestAutoSaveWithComponent("request_history")

		// Emit the frontend event
		runtime.EventsEmit(e.app.ctx, "new_transaction")
	}
}

func (e *DefaultEventEmitter) EmitStepExecuted(gleipFlowId string, stepIndex int, results []ExecutionResult) {
	if e.app != nil && e.app.ctx != nil {
		currentResults := make([]ExecutionResult, len(results))
		copy(currentResults, results)

		eventData := map[string]interface{}{
			"gleipFlowId":      gleipFlowId,
			"currentStepIndex": stepIndex,
			"results":          currentResults,
		}

		runtime.EventsEmit(e.app.ctx, "gleipFlow:stepExecuted", eventData)
		fmt.Printf("Emitted gleipFlow:stepExecuted event with %d results for step %d\n", len(currentResults), stepIndex)
	}
}

func (e *DefaultEventEmitter) EmitFuzzUpdate(stepId string, fuzzResults []FuzzResult) {
	if e.app != nil && e.app.ctx != nil {
		eventData := map[string]interface{}{
			"stepId":      stepId,
			"fuzzResults": fuzzResults,
			"isFuzzing":   true, // Flag to help frontend avoid unnecessary refreshes
		}
		runtime.EventsEmit(e.app.ctx, "gleipFlow:fuzzUpdate", eventData)
		fmt.Printf("Emitted gleipFlow:fuzzUpdate event with %d results\n", len(fuzzResults))
	}
}

// RequestExecutionOptions configures how a request should be executed
type RequestExecutionOptions struct {
	VariableProcessor  func(string) string    // Function to process variables in the request
	Timeout            *time.Duration         // Optional timeout (nil for default)
	RecalculateLength  bool                   // Whether to recalculate Content-Length
	GunzipResponse     bool                   // Whether to gunzip the response
	AdditionalMetadata map[string]interface{} // Additional metadata for result
}

// RequestExecutionResult contains the result of request execution
type RequestExecutionResult struct {
	ActualRawRequest    string // Request with variables preserved (for UI display)
	ProcessedRawRequest string // Request with variables substituted (for fuzzing display)
	Transaction         *network.HTTPTransaction
	Error               error
	ExecutionTime       time.Duration
	Metadata            map[string]interface{}
}

// executeRequestWithOptions is a generalized request execution function
func (e *GleipFlowExecutor) executeRequestWithOptions(step *RequestStep, options RequestExecutionOptions) RequestExecutionResult {
	startTime := time.Now()

	// Apply variable processing
	var processedRawRequest string
	if options.VariableProcessor != nil {
		processedRawRequest = options.VariableProcessor(step.Request.Dump)
	} else {
		processedRawRequest = step.Request.Dump
	}

	actualRawRequest := step.Request.Dump

	// Handle content length recalculation
	if options.RecalculateLength {
		_, bodyOfProcessedRequest := http_utils.SplitRawRequest(processedRawRequest)
		newContentLength := len(bodyOfProcessedRequest)
		// Update Content-Length in the original request (preserving variable placeholders) for UI display
		actualRawRequest = http_utils.UpdateContentLengthInRawRequest(actualRawRequest, newContentLength)
		// Update Content-Length in the processed request for sending
		processedRawRequest = http_utils.UpdateContentLengthInRawRequest(processedRawRequest, newContentLength)
	}

	// Create the request object for sending
	requestForSending := network.HTTPRequest{
		Host: step.Request.Host,
		TLS:  step.Request.TLS,
		Dump: processedRawRequest,
	}

	// Execute request with appropriate timeout
	var transaction *network.HTTPTransaction
	var err error

	if options.Timeout != nil {
		transaction, err = e.requestSender.SendRawRequestWithTimeout(requestForSending, options.GunzipResponse, *options.Timeout)
	} else {
		transaction, err = e.requestSender.SendRawRequest(requestForSending, options.GunzipResponse)
	}

	executionTime := time.Since(startTime)

	return RequestExecutionResult{
		ActualRawRequest:    actualRawRequest,
		ProcessedRawRequest: processedRawRequest,
		Transaction:         transaction,
		Error:               err,
		ExecutionTime:       executionTime,
		Metadata:            options.AdditionalMetadata,
	}
}

// GetAvailableVariableValuesForStep returns all variables and their values available at a given step index
// This includes gleipflow variables and variables extracted from previous steps
func (e *GleipFlowExecutor) GetAvailableVariableValuesForStep(gleipFlow *GleipFlow, stepIndex int) map[string]string {
	// Simulate execution context to get variables from previous steps
	ctx := NewExecutionContext()
	ctx.Variables = make(map[string]string)

	// Copy gleipflow variables to context (these have actual values)
	for k, v := range gleipFlow.Variables {
		ctx.Variables[k] = v
	}

	// Execute previous steps to get their extracted variables
	for i := 0; i < stepIndex && i < len(gleipFlow.Steps); i++ {
		step := gleipFlow.Steps[i]

		// Only consider selected steps for variable extraction
		if !step.Selected {
			continue
		}

		// Process different step types for variable extraction
		switch step.StepType {
		case "request":
			if step.RequestStep != nil {
				// For variables that will be extracted from requests, we can't predict their values
				// without executing the request, so we don't add them to the context
				// Only the gleipflow variables (which have actual values) will be available
			}
		case "chef":
			if step.ChefStep != nil && step.ChefStep.OutputVariable != "" {
				// For variables that will be output from chef steps, we can't predict their values
				// without executing the chef step, so we don't add them to the context
				// Only the gleipflow variables (which have actual values) will be available
			}
		case "script":
			if step.ScriptStep != nil {
				// For script steps, we can't predict what variables they'll create
				// without executing the script, so we don't add them to the context
			}
		}
	}

	return ctx.Variables
}

// GetAvailableVariablesForStep returns all variables available at a given step index
// This includes gleipflow variables and variables extracted from previous steps
func (e *GleipFlowExecutor) GetAvailableVariablesForStep(gleipFlow *GleipFlow, stepIndex int) []string {
	availableVars := make(map[string]bool)

	// Add gleipflow global variables
	for varName := range gleipFlow.Variables {
		availableVars[varName] = true
	}

	// Simulate execution context to get variables from previous steps
	ctx := NewExecutionContext()
	ctx.Variables = make(map[string]string)

	// Copy gleipflow variables to context
	for k, v := range gleipFlow.Variables {
		ctx.Variables[k] = v
	}

	// Execute previous steps to get their extracted variables
	for i := 0; i < stepIndex && i < len(gleipFlow.Steps); i++ {
		step := gleipFlow.Steps[i]

		// Only consider selected steps for variable extraction
		if !step.Selected {
			continue
		}

		// Process different step types for variable extraction
		switch step.StepType {
		case "request":
			if step.RequestStep != nil {
				// Extract variables from this request step (simulation)
				for _, extract := range step.RequestStep.VariableExtracts {
					// Add the variable name to available variables
					// We don't execute the actual request, just make the variable name available
					availableVars[extract.Name] = true
				}
			}
		case "script":
			if step.ScriptStep != nil {
				// For script steps, we can't easily predict what variables they'll create
				// without executing the script, so we'll skip this for now
				// In a future enhancement, we could parse the script for setVar() calls
			}
		case "chef":
			if step.ChefStep != nil && step.ChefStep.OutputVariable != "" {
				// Add the output variable from chef steps
				availableVars[step.ChefStep.OutputVariable] = true
			}
		}
	}

	// Convert map keys to slice and sort for deterministic order
	var result []string
	for varName := range availableVars {
		result = append(result, varName)
	}

	// Sort to ensure deterministic ordering
	sort.Strings(result)

	return result
}
