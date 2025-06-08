package backend

import (
	"Gleip/backend/chef"
	"Gleip/backend/network"
	"fmt"
)

// ScriptStep represents a script execution step in a request gleip
type ScriptStep struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"` // JavaScript code to execute
}

// FuzzResult represents the result of a single fuzz request
type FuzzResult struct {
	Word       string `json:"word"`       // The word that was used in the fuzz
	Request    string `json:"request"`    // The raw request that was sent
	Response   string `json:"response"`   // The raw response that was received
	StatusCode int    `json:"statusCode"` // HTTP status code
	Size       int    `json:"size"`       // Size of response in bytes
	Time       int64  `json:"time"`       // Time taken in milliseconds
}

// FuzzSettings represents the settings for fuzzing a request
type FuzzSettings struct {
	Delay           float64      `json:"delay"`           // Delay between requests in seconds
	CurrentWordlist []string     `json:"currentWordlist"` // List of words to fuzz with
	FuzzResults     []FuzzResult `json:"fuzzResults"`     // Results of fuzzing
}

// RequestStep represents an HTTP request step in a request gleip
type RequestStep struct {
	ID                       string               `json:"id"`
	Name                     string               `json:"name"`
	Request                  network.HTTPRequest  `json:"request"`
	Response                 network.HTTPResponse `json:"response"`
	VariableExtracts         []VariableExtract    `json:"variableExtracts"` // Variables to extract from response
	RecalculateContentLength bool                 `json:"recalculateContentLength"`
	GunzipResponse           bool                 `json:"gunzipResponse"`
	FuzzSettings             *FuzzSettings        `json:"fuzzSettings,omitempty"` // Optional fuzz settings
	IsConfigExpanded         bool                 `json:"isConfigExpanded"`       // Whether the configuration section is expanded
	IsFuzzMode               bool                 `json:"isFuzzMode"`             // Whether the step is in fuzz mode vs parse mode
}

// VariableExtract represents a variable to extract from a response
type VariableExtract struct {
	Name     string `json:"name"`     // Name of the variable to extract
	Source   string `json:"source"`   // Source of extraction (headers, body, etc.)
	Selector string `json:"selector"` // JSON path, regex, or other selector
}

// Step is an interface for gleip steps
type Step interface {
	GetID() string
	GetName() string
}

// GetID returns the ID of a ScriptStep
func (s ScriptStep) GetID() string {
	return s.ID
}

// GetName returns the name of a ScriptStep
func (s ScriptStep) GetName() string {
	return s.Name
}

// GetID returns the ID of a RequestStep
func (r RequestStep) GetID() string {
	return r.ID
}

// GetName returns the name of a RequestStep
func (r RequestStep) GetName() string {
	return r.Name
}

// GleipFlowStep represents a step in a request gleip
type GleipFlowStep struct {
	StepType    string         `json:"stepType"` // "request", "script", or "chef"
	RequestStep *RequestStep   `json:"requestStep,omitempty"`
	ScriptStep  *ScriptStep    `json:"scriptStep,omitempty"`
	ChefStep    *chef.ChefStep `json:"chefStep,omitempty"`
	Selected    bool           `json:"selected,omitempty"` // Flag for execution selection
}

// GleipFlow represents a sequence of HTTP requests and scripts
type GleipFlow struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Steps            []GleipFlowStep   `json:"steps"`
	Variables        map[string]string `json:"variables"`
	SortingIndex     int               `json:"sortingIndex"`
	ExecutionResults []ExecutionResult `json:"executionResults,omitempty"`
}

// ExecutionContext represents the context for gleip execution
type ExecutionContext struct {
	Variables map[string]string
	Results   []ExecutionResult
}

// SetVariable sets a variable in the context with debug logging
func (ctx *ExecutionContext) SetVariable(name, value string, source string) {
	oldValue, exists := ctx.Variables[name]
	ctx.Variables[name] = value

	if exists {
		fmt.Printf("VARIABLE UPDATED: %s = %s (was: %s) [source: %s]\n", name, value, oldValue, source)
	} else {
		fmt.Printf("VARIABLE CREATED: %s = %s [source: %s]\n", name, value, source)
	}
}

// ExecutionResult represents the result of executing a gleip step
type ExecutionResult struct {
	StepID           string                   `json:"stepId"`
	StepName         string                   `json:"stepName"`
	StepType         string                   `json:"stepType"`
	Success          bool                     `json:"success"`
	ErrorMessage     string                   `json:"errorMessage,omitempty"`
	Transaction      *network.HTTPTransaction `json:"transaction,omitempty"`
	Variables        map[string]string        `json:"variables,omitempty"`
	ExecutionTime    int64                    `json:"executionTime"` // in milliseconds
	ActualRawRequest string                   `json:"actualRawRequest,omitempty"`
}

// VulnerabilityType represents the type of vulnerability
type VulnerabilityType string

const (
	SQLInjection            VulnerabilityType = "sql_injection"
	XSS                     VulnerabilityType = "xss"
	CSRF                    VulnerabilityType = "csrf"
	CommandInjection        VulnerabilityType = "command_injection"
	PathTraversal           VulnerabilityType = "path_traversal"
	InsecureDeserialization VulnerabilityType = "insecure_deserialization"
	XXE                     VulnerabilityType = "xxe"
	SSRF                    VulnerabilityType = "ssrf"
	OpenRedirect            VulnerabilityType = "open_redirect"
)

// AuthConfig represents authentication configuration for automatic scanning
type AuthConfig struct {
	Type        string            `json:"type"` // "basic", "form", "token"
	Username    string            `json:"username,omitempty"`
	Password    string            `json:"password,omitempty"`
	TokenHeader string            `json:"tokenHeader,omitempty"`
	TokenValue  string            `json:"tokenValue,omitempty"`
	FormURL     string            `json:"formUrl,omitempty"`
	FormParams  map[string]string `json:"formParams,omitempty"`
}

// VulnerabilitySeverity represents the severity level of a vulnerability
type VulnerabilitySeverity string

const (
	Critical VulnerabilitySeverity = "critical"
	High     VulnerabilitySeverity = "high"
	Medium   VulnerabilitySeverity = "medium"
	Low      VulnerabilitySeverity = "low"
	Info     VulnerabilitySeverity = "info"
)

// Vulnerability represents a detected vulnerability
type Vulnerability struct {
	ID          string                `json:"id"`
	Type        VulnerabilityType     `json:"type"`
	URL         string                `json:"url"`
	Severity    VulnerabilitySeverity `json:"severity"`
	Description string                `json:"description"`
	Request     network.HTTPRequest   `json:"request"`
	Response    *network.HTTPResponse `json:"response,omitempty"`
	Timestamp   string                `json:"timestamp"`
	Payload     string                `json:"payload,omitempty"`
	Parameter   string                `json:"parameter,omitempty"`
	Details     map[string]string     `json:"details,omitempty"`
}

// NewExecutionContext creates a new execution context
func NewExecutionContext() *ExecutionContext {
	return &ExecutionContext{
		Variables: make(map[string]string),
		Results:   []ExecutionResult{},
	}
}
