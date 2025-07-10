package backend

import (
	"Gleip/backend/network"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/rbretecher/go-postman-collection"
	rt "github.com/wailsapp/wails/v2/pkg/runtime"
	"gopkg.in/yaml.v3"
)

// getHTTPStatusText returns the standard HTTP status text for a status code
func getHTTPStatusText(statusCode string) string {
	statusTexts := map[string]string{
		// 1xx Informational
		"100": "Continue",
		"101": "Switching Protocols",
		"102": "Processing",
		"103": "Early Hints",

		// 2xx Success
		"200": "OK",
		"201": "Created",
		"202": "Accepted",
		"203": "Non-Authoritative Information",
		"204": "No Content",
		"205": "Reset Content",
		"206": "Partial Content",
		"207": "Multi-Status",
		"208": "Already Reported",
		"226": "IM Used",

		// 3xx Redirection
		"300": "Multiple Choices",
		"301": "Moved Permanently",
		"302": "Found",
		"303": "See Other",
		"304": "Not Modified",
		"305": "Use Proxy",
		"307": "Temporary Redirect",
		"308": "Permanent Redirect",

		// 4xx Client Error
		"400": "Bad Request",
		"401": "Unauthorized",
		"402": "Payment Required",
		"403": "Forbidden",
		"404": "Not Found",
		"405": "Method Not Allowed",
		"406": "Not Acceptable",
		"407": "Proxy Authentication Required",
		"408": "Request Timeout",
		"409": "Conflict",
		"410": "Gone",
		"411": "Length Required",
		"412": "Precondition Failed",
		"413": "Payload Too Large",
		"414": "URI Too Long",
		"415": "Unsupported Media Type",
		"416": "Range Not Satisfiable",
		"417": "Expectation Failed",
		"418": "I'm a teapot",
		"421": "Misdirected Request",
		"422": "Unprocessable Entity",
		"423": "Locked",
		"424": "Failed Dependency",
		"425": "Too Early",
		"426": "Upgrade Required",
		"428": "Precondition Required",
		"429": "Too Many Requests",
		"431": "Request Header Fields Too Large",
		"451": "Unavailable For Legal Reasons",

		// 5xx Server Error
		"500": "Internal Server Error",
		"501": "Not Implemented",
		"502": "Bad Gateway",
		"503": "Service Unavailable",
		"504": "Gateway Timeout",
		"505": "HTTP Version Not Supported",
		"506": "Variant Also Negotiates",
		"507": "Insufficient Storage",
		"508": "Loop Detected",
		"510": "Not Extended",
		"511": "Network Authentication Required",
	}

	if text, exists := statusTexts[statusCode]; exists {
		return text
	}

	// Default fallback based on status code range
	if len(statusCode) >= 1 {
		switch statusCode[0] {
		case '1':
			return "Informational"
		case '2':
			return "Success"
		case '3':
			return "Redirection"
		case '4':
			return "Client Error"
		case '5':
			return "Server Error"
		}
	}

	return "Unknown"
}

// APICollection represents an imported API collection
type APICollection struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	Version         string              `json:"version"`
	Variables       []APIVariable       `json:"variables"`
	Requests        []APIRequest        `json:"requests"`
	FilePath        string              `json:"filePath"`
	FilePathOnDisk  string              `json:"filePathOnDisk,omitempty"`
	Type            string              `json:"type"` // "openapi2" or "openapi3"
	SecuritySchemes []APISecurityScheme `json:"securitySchemes"`
	ActiveSecurity  string              `json:"activeSecurity"` // ID of the active security scheme
}

// APIVariable represents a variable in an API collection
type APIVariable struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// APIRequest represents a request in an API collection
type APIRequest struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Method      string         `json:"method"`
	URL         string         `json:"url"`
	Path        string         `json:"path"`
	Host        string         `json:"host"`
	Headers     []APIHeader    `json:"headers"`
	Body        string         `json:"body"`
	Examples    []APIExample   `json:"examples"`
	Parameters  []APIParameter `json:"parameters"`
	Folder      string         `json:"folder,omitempty"`
}

// APIHeader represents a header in an API request
type APIHeader struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// APIExample represents an example for an API request
type APIExample struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Request  string `json:"request"`
	Response string `json:"response"`
}

// APIParameter represents a parameter in an API request
type APIParameter struct {
	Name        string `json:"name"`
	In          string `json:"in"` // "query", "path", "header", "cookie"
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// APISecurityScheme represents a security scheme defined in the OpenAPI spec
type APISecurityScheme struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "apiKey", "http", "oauth2", "openIdConnect"
	Description  string `json:"description"`
	In           string `json:"in,omitempty"`           // "query", "header", "cookie" - for apiKey
	Scheme       string `json:"scheme,omitempty"`       // "basic", "bearer", etc. - for http
	BearerFormat string `json:"bearerFormat,omitempty"` // for http with "bearer"
	KeyName      string `json:"keyName,omitempty"`      // Name of the header/query param for apiKey
	Value        string `json:"value,omitempty"`        // Current value for the security scheme
}

// ImportOpenAPICollection imports an OpenAPI collection from a file
func (a *App) ImportOpenAPICollection(filePath string) (APICollection, error) {
	fmt.Printf("Debug: Start ImportOpenAPICollection with path: %s\n", filePath)
	// Create a new collection
	collection := APICollection{
		ID:              uuid.New().String(),
		Type:            "unknown",
		Variables:       []APIVariable{},       // Initialize as empty slice, not nil
		Requests:        []APIRequest{},        // Initialize as empty slice, not nil
		SecuritySchemes: []APISecurityScheme{}, // Initialize as empty slice, not nil
	}

	// If filePath is empty, show file dialog
	if filePath == "" {
		fmt.Printf("Debug: Empty filePath, showing dialog\n")
		var err error
		filePath, err = rt.OpenFileDialog(a.ctx, rt.OpenDialogOptions{
			Title:   "Import OpenAPI Collection",
			Filters: []rt.FileFilter{{DisplayName: "OpenAPI Files (*.json, *.yaml, *.yml)", Pattern: "*.json;*.yaml;*.yml"}},
		})
		if err != nil {
			fmt.Printf("Debug: OpenFileDialog error: %v\n", err)
			return collection, fmt.Errorf("failed to open file dialog: %w", err)
		}
		if filePath == "" {
			fmt.Printf("Debug: No file selected\n")
			return collection, fmt.Errorf("no file selected")
		}
	}

	fmt.Printf("Debug: Reading file: %s\n", filePath)

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Debug: Failed to read file: %v\n", err)
		return collection, fmt.Errorf("failed to read file: %w", err)
	}

	// Create a loader to parse the OpenAPI document
	loader := openapi3.NewLoader()

	// Try to load as OpenAPI 3
	doc, err := loader.LoadFromData(data)
	if err != nil {
		fmt.Printf("Debug: Failed to load file as OpenAPI 3: %v\n", err)

		// Try to load as OpenAPI 2 (Swagger)
		var swagger2Doc openapi2.T
		if err := yaml.Unmarshal(data, &swagger2Doc); err != nil {
			// Try JSON if YAML fails
			if err := json.Unmarshal(data, &swagger2Doc); err != nil {
				fmt.Printf("Debug: Failed to load file as OpenAPI 2: %v\n", err)
				return collection, fmt.Errorf("failed to parse file %s as OpenAPI: %w", filePath, err)
			}
		}

		// Convert OpenAPI 2 to OpenAPI 3
		fmt.Printf("Debug: Converting OpenAPI 2 to OpenAPI 3\n")
		doc, err = openapi2conv.ToV3(&swagger2Doc)
		if err != nil {
			fmt.Printf("Debug: Failed to convert OpenAPI 2 to OpenAPI 3: %v\n", err)
			return collection, fmt.Errorf("failed to convert OpenAPI 2 to OpenAPI 3: %w", err)
		}

		collection.Type = "openapi2"
		collection.Version = swagger2Doc.Info.Version
	} else {
		collection.Type = "openapi3"
		collection.Version = doc.Info.Version
	}

	// Extract basic info
	collection.Name = doc.Info.Title
	collection.Description = doc.Info.Description
	collection.FilePath = filePath

	// Extract security schemes
	if doc.Components != nil && doc.Components.SecuritySchemes != nil {
		for name, schemeRef := range doc.Components.SecuritySchemes {
			if schemeRef.Value == nil {
				continue
			}

			scheme := schemeRef.Value
			securityScheme := APISecurityScheme{
				ID:          uuid.New().String(),
				Name:        name,
				Type:        string(scheme.Type),
				Description: scheme.Description,
			}

			// Handle different security scheme types
			switch scheme.Type {
			case "apiKey":
				securityScheme.In = scheme.In
				securityScheme.KeyName = scheme.Name
			case "http":
				securityScheme.Scheme = scheme.Scheme
				securityScheme.BearerFormat = scheme.BearerFormat
			}

			collection.SecuritySchemes = append(collection.SecuritySchemes, securityScheme)

			// Set the first security scheme as active by default
			if collection.ActiveSecurity == "" {
				collection.ActiveSecurity = securityScheme.ID
			}
		}
	}

	// Extract variables from schemas
	if doc.Components != nil && doc.Components.Schemas != nil {
		for name, schemaRef := range doc.Components.Schemas {
			if schemaRef.Value == nil {
				continue
			}

			schema := schemaRef.Value

			// Use example as variable value if available
			exampleValue := ""
			if schema.Example != nil {
				exampleBytes, err := json.Marshal(schema.Example)
				if err == nil {
					exampleValue = string(exampleBytes)
				}
			}

			collection.Variables = append(collection.Variables, APIVariable{
				Name:        name,
				Value:       exampleValue,
				Description: schema.Description,
			})
		}
	}

	// Get base URL from servers
	baseURL := ""
	host := ""
	basePath := ""

	if len(doc.Servers) > 0 && doc.Servers[0] != nil {
		serverURL := doc.Servers[0].URL
		baseURL = serverURL

		// Extract host and basePath
		if parsedURL, err := url.Parse(serverURL); err == nil {
			host = parsedURL.Host
			if parsedURL.Path != "" && parsedURL.Path != "/" {
				basePath = strings.TrimRight(parsedURL.Path, "/")
			}
		}
	}

	// If no host found, use a placeholder
	if host == "" {
		host = "api.example.com"
		baseURL = "https://api.example.com"
	}

	// Extract paths and create requests
	var requests []APIRequest

	// Make sure Paths is not nil
	if doc.Paths != nil {
		for path, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}

			// Process each HTTP method in the path
			operations := map[string]*openapi3.Operation{
				"GET":     pathItem.Get,
				"POST":    pathItem.Post,
				"PUT":     pathItem.Put,
				"DELETE":  pathItem.Delete,
				"OPTIONS": pathItem.Options,
				"HEAD":    pathItem.Head,
				"PATCH":   pathItem.Patch,
				"TRACE":   pathItem.Trace,
			}

			for method, operation := range operations {
				if operation == nil {
					continue
				}

				request := APIRequest{
					ID:          uuid.New().String(),
					Method:      method,
					Name:        operation.Summary,
					Description: operation.Description,
					Path:        path,
					Host:        host,
					URL:         baseURL + path,
					Headers:     []APIHeader{},    // Initialize as empty slice, not nil
					Parameters:  []APIParameter{}, // Initialize as empty slice, not nil
					Examples:    []APIExample{},   // Initialize as empty slice, not nil
				}

				// If we have a basePath, make sure it's included in the path
				if basePath != "" && !strings.HasPrefix(request.Path, basePath) {
					request.Path = basePath + path
				}

				// Extract parameters
				for _, paramRef := range operation.Parameters {
					if paramRef == nil || paramRef.Value == nil {
						continue
					}

					param := paramRef.Value
					paramType := "string"

					if param.Schema != nil && param.Schema.Value != nil && param.Schema.Value.Type != nil {
						// OpenAPI 3 represents types as an array/slice
						typeStr := param.Schema.Value.Type.Slice()
						if len(typeStr) > 0 {
							paramType = typeStr[0]
						}
					}

					request.Parameters = append(request.Parameters, APIParameter{
						Name:        param.Name,
						In:          param.In,
						Required:    param.Required,
						Type:        paramType,
						Description: param.Description,
					})
				}

				// Extract request body and headers
				if operation.RequestBody != nil && operation.RequestBody.Value != nil {
					for contentType, mediaType := range operation.RequestBody.Value.Content {
						// Add content type header
						request.Headers = append(request.Headers, APIHeader{
							Name:  "Content-Type",
							Value: contentType,
						})

						// Extract example body
						if mediaType.Example != nil {
							bodyBytes, err := json.Marshal(mediaType.Example)
							if err == nil {
								request.Body = string(bodyBytes)
							}
						} else if len(mediaType.Examples) > 0 {
							// Use the first example
							for _, exampleRef := range mediaType.Examples {
								if exampleRef != nil && exampleRef.Value != nil && exampleRef.Value.Value != nil {
									bodyBytes, err := json.Marshal(exampleRef.Value.Value)
									if err == nil {
										request.Body = string(bodyBytes)
										break
									}
								}
							}
						} else if mediaType.Schema != nil && mediaType.Schema.Value != nil && mediaType.Schema.Value.Example != nil {
							// Try to get example from schema
							bodyBytes, err := json.Marshal(mediaType.Schema.Value.Example)
							if err == nil {
								request.Body = string(bodyBytes)
							}
						}

						break // Use first content type
					}
				}

				// Extract examples from responses
				if operation.Responses != nil {
					for statusCode, responseRef := range operation.Responses.Map() {
						if responseRef == nil || responseRef.Value == nil {
							continue
						}

						response := responseRef.Value
						var responseName string
						if response.Description != nil {
							responseName = *response.Description
						} else {
							responseName = "Response " + statusCode
						}

						example := APIExample{
							ID:   uuid.New().String(),
							Name: responseName,
						}

						// Create example request
						exampleRequest := fmt.Sprintf("%s %s HTTP/1.1\nHost: %s\n", method, request.Path, host)

						for _, header := range request.Headers {
							exampleRequest += fmt.Sprintf("%s: %s\n", header.Name, header.Value)
						}

						// Add body with proper separation
						if request.Body != "" {
							exampleRequest += "\n" + request.Body
						}

						example.Request = exampleRequest

						// Create example response
						for contentType, mediaType := range response.Content {
							statusText := getHTTPStatusText(statusCode)
							responseExample := fmt.Sprintf("HTTP/1.1 %s %s\nContent-Type: %s\n", statusCode, statusText, contentType)

							// Extract example response body
							if mediaType.Example != nil {
								responseBytes, err := json.Marshal(mediaType.Example)
								if err == nil {
									responseExample += "\n" + string(responseBytes)
								}
							} else if len(mediaType.Examples) > 0 {
								// Use the first example
								for _, exampleRef := range mediaType.Examples {
									if exampleRef != nil && exampleRef.Value != nil && exampleRef.Value.Value != nil {
										responseBytes, err := json.Marshal(exampleRef.Value.Value)
										if err == nil {
											responseExample += "\n" + string(responseBytes)
											break
										}
									}
								}
							} else if mediaType.Schema != nil && mediaType.Schema.Value != nil && mediaType.Schema.Value.Example != nil {
								// Try to get example from schema
								responseBytes, err := json.Marshal(mediaType.Schema.Value.Example)
								if err == nil {
									responseExample += "\n" + string(responseBytes)
								}
							}

							example.Response = responseExample
							break // Use first content type
						}

						request.Examples = append(request.Examples, example)
					}
				}

				// Add the request to our collection
				requests = append(requests, request)
			}
		}
	}

	collection.Requests = requests

	// If no name was extracted, use the filename
	if collection.Name == "" {
		collection.Name = filepath.Base(filePath)
	}

	// Ensure security schemes are properly initialized
	if len(collection.SecuritySchemes) > 0 {
		fmt.Printf("Debug: Collection has %d security schemes\n", len(collection.SecuritySchemes))
		// Make sure active security is set
		if collection.ActiveSecurity == "" {
			collection.ActiveSecurity = collection.SecuritySchemes[0].ID
			fmt.Printf("Debug: Setting first security scheme as active: %s (%s)\n",
				collection.SecuritySchemes[0].Name, collection.SecuritySchemes[0].ID)
		} else {
			fmt.Printf("Debug: Collection already has active security: %s\n", collection.ActiveSecurity)
			// Verify the active security ID points to a valid scheme
			found := false
			for _, scheme := range collection.SecuritySchemes {
				if scheme.ID == collection.ActiveSecurity {
					found = true
					fmt.Printf("Debug: Found matching scheme for active security: %s\n", scheme.Name)
					break
				}
			}

			// If not found, reset to first scheme
			if !found && len(collection.SecuritySchemes) > 0 {
				fmt.Printf("Debug: Active security ID doesn't match any scheme, resetting to first scheme\n")
				collection.ActiveSecurity = collection.SecuritySchemes[0].ID
			}
		}
	} else {
		fmt.Printf("Debug: Collection has no security schemes\n")
	}

	fmt.Printf("Debug: Collection parsed successfully. Name: %s, Requests: %d\n",
		collection.Name, len(collection.Requests))

	// Save the collection to the project
	fmt.Printf("Debug: Saving collection to project\n")
	collectionID, err := a.SaveAPICollection(collection)
	if err != nil {
		fmt.Printf("Debug: SaveAPICollection error: %v\n", err)
		return collection, fmt.Errorf("failed to save imported collection: %w", err)
	}
	fmt.Printf("Debug: Collection saved with ID: %s\n", collectionID)

	// Set this as the selected API collection
	fmt.Printf("Debug: Setting as selected API collection\n")
	if err := a.SetSelectedAPICollection(collection.ID); err != nil {
		fmt.Printf("Debug: Failed to set selected API collection: %v\n", err)
	}

	fmt.Printf("Debug: ImportOpenAPICollection completed successfully\n")
	return collection, nil
}

// SaveAPICollection saves an API collection to the project
func (a *App) SaveAPICollection(collection APICollection) (string, error) {
	fmt.Printf("Debug: SaveAPICollection started for collection: %s\n", collection.Name)

	a.projectMutex.Lock()
	fmt.Printf("Debug: Acquired projectMutex lock\n")

	a.apiCollectionsMutex.Lock()
	fmt.Printf("Debug: Acquired apiCollectionsMutex lock\n")

	// We'll handle unlocking manually to avoid deadlocks

	// Check if the project exists - never create one here
	if a.currentProject == nil {
		a.projectMutex.Unlock()
		a.apiCollectionsMutex.Unlock()
		fmt.Printf("Debug: ERROR - currentProject is nil\n")
		return "", fmt.Errorf("cannot save collection: no active project. This may indicate an application initialization issue")
	}

	fmt.Printf("Debug: Current project exists, ID: %s, Name: %s\n",
		a.currentProject.ID, a.currentProject.Name)
	fmt.Printf("Debug: Current project has %d collections\n",
		len(a.currentProject.APICollections))

	// Check if collection with this ID already exists
	for i, existingCollection := range a.currentProject.APICollections {
		if existingCollection.ID == collection.ID {
			fmt.Printf("Debug: Found existing collection with same ID, updating\n")
			// Update existing collection
			a.currentProject.APICollections[i] = collection

			// Release locks
			a.projectMutex.Unlock()
			a.apiCollectionsMutex.Unlock()
			fmt.Printf("Debug: Released locks before auto-save\n")

			// Request auto-save
			a.requestAutoSaveWithComponent("api_collections")
			fmt.Printf("Debug: Auto-save requested after collection update\n")

			return collection.ID, nil
		}
	}

	// Add new collection
	fmt.Printf("Debug: Adding new collection to project\n")
	a.currentProject.APICollections = append(a.currentProject.APICollections, collection)

	// Release locks
	a.projectMutex.Unlock()
	a.apiCollectionsMutex.Unlock()
	fmt.Printf("Debug: Released locks before auto-save\n")

	// Request auto-save
	a.requestAutoSaveWithComponent("api_collections")
	fmt.Printf("Debug: Auto-save requested after adding new collection\n")

	// Track collection creation event
	TrackAPICollectionCreated(collection.Name)
	fmt.Printf("Debug: Tracked collection creation event\n")

	fmt.Printf("Debug: SaveAPICollection completed successfully\n")
	return collection.ID, nil
}

// GetAPICollections returns all API collections in the current project
func (a *App) GetAPICollections() ([]APICollection, error) {
	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	// Check if the project exists
	if a.currentProject == nil {
		return []APICollection{}, nil
	}

	// Return a copy of the collections
	collections := make([]APICollection, len(a.currentProject.APICollections))
	copy(collections, a.currentProject.APICollections)

	return collections, nil
}

// GetAPICollection returns a specific API collection by ID from the current project
func (a *App) GetAPICollection(id string) (APICollection, error) {
	a.projectMutex.RLock()
	defer a.projectMutex.RUnlock()

	// Check if the project exists
	if a.currentProject == nil {
		return APICollection{}, fmt.Errorf("cannot get collection: no active project. Please try restarting the application")
	}

	// Find the collection
	for _, collection := range a.currentProject.APICollections {
		if collection.ID == id {
			return collection, nil
		}
	}

	return APICollection{}, fmt.Errorf("collection not found: %s", id)
}

// DeleteAPICollection deletes an API collection from the current project
func (a *App) DeleteAPICollection(id string) error {
	a.projectMutex.Lock()
	a.apiCollectionsMutex.Lock()

	// We'll handle unlocking manually to avoid deadlocks

	// Check if the project exists
	if a.currentProject == nil {
		a.projectMutex.Unlock()
		a.apiCollectionsMutex.Unlock()
		return fmt.Errorf("cannot delete collection: no active project. Please try restarting the application")
	}

	// Get collection name for telemetry
	var collectionName string
	for _, collection := range a.currentProject.APICollections {
		if collection.ID == id {
			collectionName = collection.Name
			break
		}
	}

	// Find and remove the collection
	for i, collection := range a.currentProject.APICollections {
		if collection.ID == id {
			// Remove the collection
			a.currentProject.APICollections = append(a.currentProject.APICollections[:i], a.currentProject.APICollections[i+1:]...)

			// Release locks
			a.projectMutex.Unlock()
			a.apiCollectionsMutex.Unlock()

			// Request auto-save
			a.requestAutoSaveWithComponent("api_collections")

			// Track collection deletion
			if collectionName != "" {
				TrackAPICollectionDeleted(collectionName)
			} else {
				TrackAPICollectionDeleted(id)
			}

			return nil
		}
	}

	a.projectMutex.Unlock()
	a.apiCollectionsMutex.Unlock()
	return fmt.Errorf("collection not found: %s", id)
}

// SetAPICollectionSecurity sets the active security scheme for a collection
func (a *App) SetAPICollectionSecurity(collectionID, securityID, securityValue string) error {
	fmt.Printf("Debug: SetAPICollectionSecurity called - collection: %s, security: %s, value: %s\n", collectionID, securityID, securityValue)

	a.projectMutex.Lock()
	a.apiCollectionsMutex.Lock()

	// We'll handle unlocking manually to avoid deadlocks

	// Check if the project exists
	if a.currentProject == nil {
		a.projectMutex.Unlock()
		a.apiCollectionsMutex.Unlock()
		return fmt.Errorf("cannot set security: no active project. Please try restarting the application")
	}

	// Find the collection
	for i, collection := range a.currentProject.APICollections {
		if collection.ID == collectionID {
			// Set the active security scheme
			oldActiveSecurity := a.currentProject.APICollections[i].ActiveSecurity
			a.currentProject.APICollections[i].ActiveSecurity = securityID
			fmt.Printf("Debug: Changed active security from %s to %s\n", oldActiveSecurity, securityID)

			// Update the security scheme value
			for j, scheme := range a.currentProject.APICollections[i].SecuritySchemes {
				if scheme.ID == securityID {
					a.currentProject.APICollections[i].SecuritySchemes[j].Value = securityValue
					fmt.Printf("Debug: Updated security scheme value for %s\n", scheme.Name)
					break
				}
			}

			// Get updated collection for event
			updatedCollection := a.currentProject.APICollections[i]

			// Release locks
			a.projectMutex.Unlock()
			a.apiCollectionsMutex.Unlock()

			// Request auto-save
			a.requestAutoSaveWithComponent("api_collections")

			// Emit event to notify frontend that security has changed
			// This will trigger the frontend to refresh all visible requests
			rt.EventsEmit(a.ctx, "api-collection:security-changed", map[string]interface{}{
				"collectionID": collectionID,
				"securityID":   securityID,
				"collection":   updatedCollection,
			})
			fmt.Printf("Debug: Emitted security-changed event\n")

			return nil
		}
	}

	a.projectMutex.Unlock()
	a.apiCollectionsMutex.Unlock()
	return fmt.Errorf("collection not found: %s", collectionID)
}

// GetRequestWithSecurity returns a request with the appropriate security headers/params applied
func (a *App) GetRequestWithSecurity(collectionID, requestID string) (string, error) {
	fmt.Printf("Debug: Getting request with security, collection: %s, request: %s\n", collectionID, requestID)
	// Get the collection
	collection, err := a.GetAPICollection(collectionID)
	if err != nil {
		fmt.Printf("Debug: Failed to get collection: %v\n", err)
		return "", fmt.Errorf("failed to get collection: %w", err)
	}

	// Find the request
	var targetRequest APIRequest
	found := false
	for _, req := range collection.Requests {
		if req.ID == requestID {
			targetRequest = req
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Debug: Request not found: %s\n", requestID)
		return "", fmt.Errorf("request not found: %s", requestID)
	}

	// Find the active security scheme
	var activeScheme *APISecurityScheme
	for _, scheme := range collection.SecuritySchemes {
		if scheme.ID == collection.ActiveSecurity {
			activeScheme = &scheme
			fmt.Printf("Debug: Found active security scheme: %s\n", scheme.Name)
			break
		}
	}

	// If no examples, create a basic request using the target request's data
	if len(targetRequest.Examples) == 0 {
		fmt.Printf("Debug: No examples found, building request manually\n")
		// Build a raw request manually
		method := targetRequest.Method
		path := targetRequest.Path
		host := targetRequest.Host

		// Start with request line
		rawRequest := fmt.Sprintf("%s %s HTTP/1.1\n", method, path)

		// Add host header
		if host != "" {
			rawRequest += fmt.Sprintf("Host: %s\n", host)
		}

		// Add other headers
		for _, header := range targetRequest.Headers {
			rawRequest += fmt.Sprintf("%s: %s\n", header.Name, header.Value)
		}

		// Add body with proper separation
		rawRequest += "\n" + targetRequest.Body

		// If we have a security scheme, apply it to this request
		if activeScheme != nil {
			fmt.Printf("Debug: Applying security scheme to manually built request\n")
			return a.applySecurityToRequest(rawRequest, *activeScheme)
		}

		fmt.Printf("Debug: No security scheme to apply\n")
		return rawRequest, nil
	}

	// If we have examples but no active security scheme, return the first example as is
	if activeScheme == nil {
		fmt.Printf("Debug: No active security scheme, returning first example as is\n")
		return targetRequest.Examples[0].Request, nil
	}

	// Get the request from the first example
	baseRequest := targetRequest.Examples[0].Request
	fmt.Printf("Debug: Applying security scheme to example request\n")

	// Apply security to the request - always apply, even if value is empty
	modifiedRequest, err := a.applySecurityToRequest(baseRequest, *activeScheme)
	if err != nil {
		fmt.Printf("Debug: Failed to apply security: %v\n", err)
		return "", fmt.Errorf("failed to apply security: %w", err)
	}

	fmt.Printf("Debug: Successfully applied security to request\n")
	return modifiedRequest, nil
}

// GetRequestExamplesWithSecurity returns all examples of a request with security applied
func (a *App) GetRequestExamplesWithSecurity(collectionID, requestID string) ([]APIExample, error) {
	fmt.Printf("Debug: Getting request examples with security, collection: %s, request: %s\n", collectionID, requestID)
	// Get the collection
	collection, err := a.GetAPICollection(collectionID)
	if err != nil {
		fmt.Printf("Debug: Failed to get collection: %v\n", err)
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}

	// Find the request
	var targetRequest APIRequest
	found := false
	for _, req := range collection.Requests {
		if req.ID == requestID {
			targetRequest = req
			found = true
			break
		}
	}

	if !found {
		fmt.Printf("Debug: Request not found: %s\n", requestID)
		return nil, fmt.Errorf("request not found: %s", requestID)
	}

	// If no examples, create a basic example using the target request's data
	if len(targetRequest.Examples) == 0 {
		fmt.Printf("Debug: No examples found, creating synthetic example\n")
		// Build a raw request manually
		method := targetRequest.Method
		path := targetRequest.Path
		host := targetRequest.Host

		// Start with request line
		rawRequest := fmt.Sprintf("%s %s HTTP/1.1\n", method, path)

		// Add host header
		if host != "" {
			rawRequest += fmt.Sprintf("Host: %s\n", host)
		}

		// Add other headers
		for _, header := range targetRequest.Headers {
			rawRequest += fmt.Sprintf("%s: %s\n", header.Name, header.Value)
		}

		// Add body with proper separation
		rawRequest += "\n" + targetRequest.Body

		// Create a synthetic example
		synthetic := APIExample{
			ID:       uuid.New().String(),
			Name:     "Generated Example",
			Request:  rawRequest,
			Response: "", // No response available
		}

		// We'll return the example without applying security here
		// The security will be applied in the code handling the returned example

		return []APIExample{synthetic}, nil
	}

	// Find the active security scheme
	var activeScheme *APISecurityScheme
	for _, scheme := range collection.SecuritySchemes {
		if scheme.ID == collection.ActiveSecurity {
			activeScheme = &scheme
			fmt.Printf("Debug: Found active security scheme: %s\n", scheme.Name)
			break
		}
	}

	// If no active scheme, return the examples as is
	if activeScheme == nil {
		fmt.Printf("Debug: No active security scheme, returning examples as is\n")
		return targetRequest.Examples, nil
	}

	// Apply security to all examples
	fmt.Printf("Debug: Applying security to all examples\n")
	modifiedExamples := make([]APIExample, len(targetRequest.Examples))
	for i, example := range targetRequest.Examples {
		// Create a copy of the example
		modifiedExample := example

		// Apply security to the request - always apply, even if value is empty
		modifiedRequest, err := a.applySecurityToRequest(example.Request, *activeScheme)
		if err != nil {
			// If there's an error with one example, continue with others
			fmt.Printf("Debug: Failed to apply security to example %s: %v\n", example.Name, err)
			modifiedExample.Request = example.Request
		} else {
			modifiedExample.Request = modifiedRequest
		}

		modifiedExamples[i] = modifiedExample
	}

	fmt.Printf("Debug: Returning %d modified examples\n", len(modifiedExamples))
	return modifiedExamples, nil
}

// applySecurityToRequest applies a security scheme to a raw HTTP request string
func (a *App) applySecurityToRequest(request string, scheme APISecurityScheme) (string, error) {
	// Parse the request to modify it
	lines := strings.Split(request, "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("invalid request format")
	}

	// Apply security based on type
	switch scheme.Type {
	case "apiKey":
		if scheme.In == "header" {
			// Check if the header already exists and update it
			headerExists := false
			for i, line := range lines {
				if strings.HasPrefix(line, scheme.KeyName+":") {
					lines[i] = fmt.Sprintf("%s: %s", scheme.KeyName, scheme.Value)
					headerExists = true
					break
				}
			}

			// If header doesn't exist, add it after the request line and Host
			if !headerExists {
				// Find where to insert (after Host header)
				insertIdx := 1 // Default after first line
				for i, line := range lines {
					if strings.HasPrefix(line, "Host:") {
						insertIdx = i + 1
						break
					}
				}

				// Create a new slice with the new header
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:insertIdx]...)
				newLines = append(newLines, fmt.Sprintf("%s: %s", scheme.KeyName, scheme.Value))
				newLines = append(newLines, lines[insertIdx:]...)
				lines = newLines
			}
		} else if scheme.In == "query" {
			// Add query parameter to URL
			requestLine := lines[0]
			parts := strings.Split(requestLine, " ")
			if len(parts) >= 2 {
				url := parts[1]
				separator := "?"
				if strings.Contains(url, "?") {
					separator = "&"
				}
				url = fmt.Sprintf("%s%s%s=%s", url, separator, scheme.KeyName, scheme.Value)
				parts[1] = url
				lines[0] = strings.Join(parts, " ")
			}
		}
	case "http":
		if scheme.Scheme == "basic" {
			// Add Basic authentication header
			headerExists := false
			for i, line := range lines {
				if strings.HasPrefix(line, "Authorization:") {
					lines[i] = fmt.Sprintf("Authorization: Basic %s", scheme.Value)
					headerExists = true
					break
				}
			}

			if !headerExists {
				// Find where to insert (after Host header)
				insertIdx := 1 // Default after first line
				for i, line := range lines {
					if strings.HasPrefix(line, "Host:") {
						insertIdx = i + 1
						break
					}
				}

				// Create a new slice with the new header
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:insertIdx]...)
				newLines = append(newLines, fmt.Sprintf("Authorization: Basic %s", scheme.Value))
				newLines = append(newLines, lines[insertIdx:]...)
				lines = newLines
			}
		} else if scheme.Scheme == "bearer" {
			// Add Bearer authentication header
			headerExists := false
			for i, line := range lines {
				if strings.HasPrefix(line, "Authorization:") {
					lines[i] = fmt.Sprintf("Authorization: Bearer %s", scheme.Value)
					headerExists = true
					break
				}
			}

			if !headerExists {
				// Find where to insert (after Host header)
				insertIdx := 1 // Default after first line
				for i, line := range lines {
					if strings.HasPrefix(line, "Host:") {
						insertIdx = i + 1
						break
					}
				}

				// Create a new slice with the new header
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:insertIdx]...)
				newLines = append(newLines, fmt.Sprintf("Authorization: Bearer %s", scheme.Value))
				newLines = append(newLines, lines[insertIdx:]...)
				lines = newLines
			}
		}
	}

	// Reconstruct the request
	return strings.Join(lines, "\n"), nil
}

// ImportOpenAPICollectionAsync initiates an asynchronous import of an OpenAPI collection
// It returns immediately with an ID that can be used to check the import status
func (a *App) ImportOpenAPICollectionAsync(filePath string) (string, error) {
	// Generate an import ID that will be used to track this import operation
	importID := uuid.New().String()

	// Start the import in a background goroutine
	go func() {
		collection, err := a.performImport(filePath)
		if err != nil {
			// Emit an error event for the frontend
			rt.EventsEmit(a.ctx, "api-collection:import-error", map[string]interface{}{
				"importID": importID,
				"error":    err.Error(),
			})
			return
		}

		// Emit a success event with the collection data
		rt.EventsEmit(a.ctx, "api-collection:import-success", map[string]interface{}{
			"importID":   importID,
			"collection": collection,
		})
	}()

	return importID, nil
}

// performImport does the actual import work without blocking the UI
func (a *App) performImport(filePath string) (APICollection, error) {
	// Create a new collection
	collection := APICollection{
		ID:              uuid.New().String(),
		Type:            "unknown",
		Variables:       []APIVariable{},       // Initialize as empty slice, not nil
		Requests:        []APIRequest{},        // Initialize as empty slice, not nil
		SecuritySchemes: []APISecurityScheme{}, // Initialize as empty slice, not nil
	}

	// If filePath is empty, return an error - path should already be selected
	if filePath == "" {
		return collection, fmt.Errorf("no file path provided")
	}

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Debug: Failed to read file: %v\n", err)
		return collection, fmt.Errorf("failed to read file: %w", err)
	}

	// Create a loader to parse the OpenAPI document
	loader := openapi3.NewLoader()

	// Try to load as OpenAPI 3
	doc, err := loader.LoadFromData(data)
	if err != nil {
		fmt.Printf("Debug: Failed to load file as OpenAPI 3: %v\n", err)

		// Try to load as OpenAPI 2 (Swagger)
		var swagger2Doc openapi2.T
		if err := yaml.Unmarshal(data, &swagger2Doc); err != nil {
			// Try JSON if YAML fails
			if err := json.Unmarshal(data, &swagger2Doc); err != nil {
				fmt.Printf("Debug: Failed to load file as OpenAPI 2: %v\n", err)
				return collection, fmt.Errorf("failed to parse file %s as OpenAPI: %w", filePath, err)
			}
		}

		// Convert OpenAPI 2 to OpenAPI 3
		fmt.Printf("Debug: Converting OpenAPI 2 to OpenAPI 3\n")
		doc, err = openapi2conv.ToV3(&swagger2Doc)
		if err != nil {
			fmt.Printf("Debug: Failed to convert OpenAPI 2 to OpenAPI 3: %v\n", err)
			return collection, fmt.Errorf("failed to convert OpenAPI 2 to OpenAPI 3: %w", err)
		}

		collection.Type = "openapi2"
		collection.Version = swagger2Doc.Info.Version
	} else {
		collection.Type = "openapi3"
		collection.Version = doc.Info.Version
	}

	// Extract basic info
	collection.Name = doc.Info.Title
	collection.Description = doc.Info.Description
	collection.FilePath = filePath

	// Extract security schemes
	if doc.Components != nil && doc.Components.SecuritySchemes != nil {
		for name, schemeRef := range doc.Components.SecuritySchemes {
			if schemeRef.Value == nil {
				continue
			}

			scheme := schemeRef.Value
			securityScheme := APISecurityScheme{
				ID:          uuid.New().String(),
				Name:        name,
				Type:        string(scheme.Type),
				Description: scheme.Description,
			}

			// Handle different security scheme types
			switch scheme.Type {
			case "apiKey":
				securityScheme.In = scheme.In
				securityScheme.KeyName = scheme.Name
			case "http":
				securityScheme.Scheme = scheme.Scheme
				securityScheme.BearerFormat = scheme.BearerFormat
			}

			collection.SecuritySchemes = append(collection.SecuritySchemes, securityScheme)

			// Set the first security scheme as active by default
			if collection.ActiveSecurity == "" {
				collection.ActiveSecurity = securityScheme.ID
			}
		}
	}

	// Extract variables from schemas
	if doc.Components != nil && doc.Components.Schemas != nil {
		for name, schemaRef := range doc.Components.Schemas {
			if schemaRef.Value == nil {
				continue
			}

			schema := schemaRef.Value

			// Use example as variable value if available
			exampleValue := ""
			if schema.Example != nil {
				exampleBytes, err := json.Marshal(schema.Example)
				if err == nil {
					exampleValue = string(exampleBytes)
				}
			}

			collection.Variables = append(collection.Variables, APIVariable{
				Name:        name,
				Value:       exampleValue,
				Description: schema.Description,
			})
		}
	}

	// Get base URL from servers
	baseURL := ""
	host := ""
	basePath := ""

	if len(doc.Servers) > 0 && doc.Servers[0] != nil {
		serverURL := doc.Servers[0].URL
		baseURL = serverURL

		// Extract host and basePath
		if parsedURL, err := url.Parse(serverURL); err == nil {
			host = parsedURL.Host
			if parsedURL.Path != "" && parsedURL.Path != "/" {
				basePath = strings.TrimRight(parsedURL.Path, "/")
			}
		}
	}

	// If no host found, use a placeholder
	if host == "" {
		host = "api.example.com"
		baseURL = "https://api.example.com"
	}

	// Extract paths and create requests
	var requests []APIRequest

	// Make sure Paths is not nil
	if doc.Paths != nil {
		for path, pathItem := range doc.Paths.Map() {
			if pathItem == nil {
				continue
			}

			// Process each HTTP method in the path
			operations := map[string]*openapi3.Operation{
				"GET":     pathItem.Get,
				"POST":    pathItem.Post,
				"PUT":     pathItem.Put,
				"DELETE":  pathItem.Delete,
				"OPTIONS": pathItem.Options,
				"HEAD":    pathItem.Head,
				"PATCH":   pathItem.Patch,
				"TRACE":   pathItem.Trace,
			}

			for method, operation := range operations {
				if operation == nil {
					continue
				}

				request := APIRequest{
					ID:          uuid.New().String(),
					Method:      method,
					Name:        operation.Summary,
					Description: operation.Description,
					Path:        path,
					Host:        host,
					URL:         baseURL + path,
					Headers:     []APIHeader{},    // Initialize as empty slice, not nil
					Parameters:  []APIParameter{}, // Initialize as empty slice, not nil
					Examples:    []APIExample{},   // Initialize as empty slice, not nil
				}

				// If we have a basePath, make sure it's included in the path
				if basePath != "" && !strings.HasPrefix(request.Path, basePath) {
					request.Path = basePath + path
				}

				// Extract parameters
				for _, paramRef := range operation.Parameters {
					if paramRef == nil || paramRef.Value == nil {
						continue
					}

					param := paramRef.Value
					paramType := "string"

					if param.Schema != nil && param.Schema.Value != nil && param.Schema.Value.Type != nil {
						// OpenAPI 3 represents types as an array/slice
						typeStr := param.Schema.Value.Type.Slice()
						if len(typeStr) > 0 {
							paramType = typeStr[0]
						}
					}

					request.Parameters = append(request.Parameters, APIParameter{
						Name:        param.Name,
						In:          param.In,
						Required:    param.Required,
						Type:        paramType,
						Description: param.Description,
					})
				}

				// Extract request body and headers
				if operation.RequestBody != nil && operation.RequestBody.Value != nil {
					for contentType, mediaType := range operation.RequestBody.Value.Content {
						// Add content type header
						request.Headers = append(request.Headers, APIHeader{
							Name:  "Content-Type",
							Value: contentType,
						})

						// Extract example body
						if mediaType.Example != nil {
							bodyBytes, err := json.Marshal(mediaType.Example)
							if err == nil {
								request.Body = string(bodyBytes)
							}
						} else if len(mediaType.Examples) > 0 {
							// Use the first example
							for _, exampleRef := range mediaType.Examples {
								if exampleRef != nil && exampleRef.Value != nil && exampleRef.Value.Value != nil {
									bodyBytes, err := json.Marshal(exampleRef.Value.Value)
									if err == nil {
										request.Body = string(bodyBytes)
										break
									}
								}
							}
						} else if mediaType.Schema != nil && mediaType.Schema.Value != nil && mediaType.Schema.Value.Example != nil {
							// Try to get example from schema
							bodyBytes, err := json.Marshal(mediaType.Schema.Value.Example)
							if err == nil {
								request.Body = string(bodyBytes)
							}
						}

						break // Use first content type
					}
				}

				// Extract examples from responses
				if operation.Responses != nil {
					for statusCode, responseRef := range operation.Responses.Map() {
						if responseRef == nil || responseRef.Value == nil {
							continue
						}

						response := responseRef.Value
						var responseName string
						if response.Description != nil {
							responseName = *response.Description
						} else {
							responseName = "Response " + statusCode
						}

						example := APIExample{
							ID:   uuid.New().String(),
							Name: responseName,
						}

						// Create example request
						exampleRequest := fmt.Sprintf("%s %s HTTP/1.1\nHost: %s\n", method, request.Path, host)

						for _, header := range request.Headers {
							exampleRequest += fmt.Sprintf("%s: %s\n", header.Name, header.Value)
						}

						// Add body with proper separation
						if request.Body != "" {
							exampleRequest += "\n" + request.Body
						}

						example.Request = exampleRequest

						// Create example response
						for contentType, mediaType := range response.Content {
							statusText := getHTTPStatusText(statusCode)
							responseExample := fmt.Sprintf("HTTP/1.1 %s %s\nContent-Type: %s\n", statusCode, statusText, contentType)

							// Extract example response body
							if mediaType.Example != nil {
								responseBytes, err := json.Marshal(mediaType.Example)
								if err == nil {
									responseExample += "\n" + string(responseBytes)
								}
							} else if len(mediaType.Examples) > 0 {
								// Use the first example
								for _, exampleRef := range mediaType.Examples {
									if exampleRef != nil && exampleRef.Value != nil && exampleRef.Value.Value != nil {
										responseBytes, err := json.Marshal(exampleRef.Value.Value)
										if err == nil {
											responseExample += "\n" + string(responseBytes)
											break
										}
									}
								}
							} else if mediaType.Schema != nil && mediaType.Schema.Value != nil && mediaType.Schema.Value.Example != nil {
								// Try to get example from schema
								responseBytes, err := json.Marshal(mediaType.Schema.Value.Example)
								if err == nil {
									responseExample += "\n" + string(responseBytes)
								}
							}

							example.Response = responseExample
							break // Use first content type
						}

						request.Examples = append(request.Examples, example)
					}
				}

				// Add the request to our collection
				requests = append(requests, request)
			}
		}
	}

	collection.Requests = requests

	// If no name was extracted, use the filename
	if collection.Name == "" {
		collection.Name = filepath.Base(filePath)
	}

	// Ensure security schemes are properly initialized
	if len(collection.SecuritySchemes) > 0 {
		fmt.Printf("Debug: Collection has %d security schemes\n", len(collection.SecuritySchemes))
		// Make sure active security is set
		if collection.ActiveSecurity == "" {
			collection.ActiveSecurity = collection.SecuritySchemes[0].ID
			fmt.Printf("Debug: Setting first security scheme as active: %s (%s)\n",
				collection.SecuritySchemes[0].Name, collection.SecuritySchemes[0].ID)
		} else {
			fmt.Printf("Debug: Collection already has active security: %s\n", collection.ActiveSecurity)
			// Verify the active security ID points to a valid scheme
			found := false
			for _, scheme := range collection.SecuritySchemes {
				if scheme.ID == collection.ActiveSecurity {
					found = true
					fmt.Printf("Debug: Found matching scheme for active security: %s\n", scheme.Name)
					break
				}
			}

			// If not found, reset to first scheme
			if !found && len(collection.SecuritySchemes) > 0 {
				fmt.Printf("Debug: Active security ID doesn't match any scheme, resetting to first scheme\n")
				collection.ActiveSecurity = collection.SecuritySchemes[0].ID
			}
		}
	} else {
		fmt.Printf("Debug: Collection has no security schemes\n")
	}

	fmt.Printf("Debug: Collection parsed successfully. Name: %s, Requests: %d\n",
		collection.Name, len(collection.Requests))

	// Save the collection to the project
	fmt.Printf("Debug: Saving collection to project\n")
	collectionID, err := a.SaveAPICollection(collection)
	if err != nil {
		fmt.Printf("Debug: SaveAPICollection error: %v\n", err)
		return collection, fmt.Errorf("failed to save imported collection: %w", err)
	}
	fmt.Printf("Debug: Collection saved with ID: %s\n", collectionID)

	// Set this as the selected API collection
	fmt.Printf("Debug: Setting as selected API collection\n")
	if err := a.SetSelectedAPICollection(collection.ID); err != nil {
		fmt.Printf("Debug: Failed to set selected API collection: %v\n", err)
	}

	fmt.Printf("Debug: ImportOpenAPICollection completed successfully\n")
	return collection, nil
}

// CopyAPIRequestToClipboard copies an API collection request to the system clipboard
func (a *App) CopyAPIRequestToClipboard(requestID string) error {
	fmt.Printf("Debug: Copying API request to clipboard, request: %s\n", requestID)

	// Find the collection and request across all collections
	var foundCollection *APICollection
	var foundRequest *APIRequest

	// Get all collections
	collections, err := a.GetAPICollections()
	if err != nil {
		return fmt.Errorf("failed to get API collections: %w", err)
	}

	// Search for the request in all collections
	for i := range collections {
		for j := range collections[i].Requests {
			if collections[i].Requests[j].ID == requestID {
				foundCollection = &collections[i]
				foundRequest = &collections[i].Requests[j]
				break
			}
		}
		if foundRequest != nil {
			break
		}
	}

	if foundRequest == nil {
		return fmt.Errorf("API request not found: %s", requestID)
	}

	// Track request copying to clipboard from API collection
	requestURL := foundRequest.URL
	if requestURL == "" {
		// Build URL from host and path if URL is empty
		if foundRequest.Host != "" && foundRequest.Path != "" {
			if strings.HasPrefix(foundRequest.Path, "/") {
				requestURL = "https://" + foundRequest.Host + foundRequest.Path
			} else {
				requestURL = "https://" + foundRequest.Host + "/" + foundRequest.Path
			}
		} else {
			requestURL = foundRequest.Path // fallback to just the path
		}
	}
	TrackRequestCopiedToClipboard(foundRequest.Method, requestURL, "api_collection")

	// Get the request with security applied if available
	var rawRequest string
	if foundCollection.ActiveSecurity != "" {
		req, err := a.GetRequestWithSecurity(foundCollection.ID, requestID)
		if err == nil {
			rawRequest = req
			fmt.Printf("Debug: Successfully applied security to request for clipboard\n")
		} else {
			fmt.Printf("Debug: Failed to apply security for clipboard: %v\n", err)
		}
	}

	// If we couldn't get the request with security, build it without security
	if rawRequest == "" {
		fmt.Printf("Debug: Building raw request without security for clipboard\n")
		// Build a raw HTTP request
		method := foundRequest.Method
		path := foundRequest.Path
		var headerLines []string

		// Add headers from the request (Host header should already be included)
		for _, header := range foundRequest.Headers {
			headerLines = append(headerLines, fmt.Sprintf("%s: %s", header.Name, header.Value))
		}

		// Build the raw request
		rawRequest = fmt.Sprintf("%s %s HTTP/1.1\r\n%s\r\n\r\n%s",
			method,
			path,
			strings.Join(headerLines, "\r\n"),
			foundRequest.Body)
	}

	// Determine if TLS is being used
	isTLS := strings.HasPrefix(requestURL, "https://")

	// Create HTTPRequest object in the proper format
	httpRequest := network.HTTPRequest{
		Host: foundRequest.Host,
		Dump: rawRequest,
		TLS:  isTLS,
	}

	// Serialize to JSON for the clipboard
	serialized, err := json.MarshalIndent(httpRequest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize request: %v", err)
	}

	// Set to system clipboard using the wails runtime
	rt.ClipboardSetText(a.ctx, string(serialized))

	fmt.Printf("Debug: Successfully copied API request to clipboard\n")
	return nil
}

// detectCollectionType attempts to determine if a file is an OpenAPI spec or Postman collection
func detectCollectionType(data []byte) string {
	// Try to parse as JSON first
	var jsonData map[string]interface{}
	if err := json.Unmarshal(data, &jsonData); err == nil {
		// Check for Postman collection indicators
		if info, exists := jsonData["info"]; exists {
			if infoMap, ok := info.(map[string]interface{}); ok {
				if schema, exists := infoMap["schema"]; exists {
					if schemaStr, ok := schema.(string); ok && strings.Contains(schemaStr, "postman") {
						return "postman"
					}
				}
				// Also check for _postman_id which is unique to Postman
				if _, exists := infoMap["_postman_id"]; exists {
					return "postman"
				}
			}
		}

		// Check for OpenAPI indicators
		if openapi, exists := jsonData["openapi"]; exists {
			if _, ok := openapi.(string); ok {
				return "openapi3"
			}
		}
		if swagger, exists := jsonData["swagger"]; exists {
			if _, ok := swagger.(string); ok {
				return "openapi2"
			}
		}
	}

	// Try to parse as YAML
	var yamlData map[string]interface{}
	if err := yaml.Unmarshal(data, &yamlData); err == nil {
		// Check for OpenAPI indicators in YAML
		if openapi, exists := yamlData["openapi"]; exists {
			if _, ok := openapi.(string); ok {
				return "openapi3"
			}
		}
		if swagger, exists := yamlData["swagger"]; exists {
			if _, ok := swagger.(string); ok {
				return "openapi2"
			}
		}
	}

	return "unknown"
}

// ImportAPICollection imports either an OpenAPI specification or Postman collection
func (a *App) ImportAPICollection(filePath string) (APICollection, error) {
	fmt.Printf("Debug: Start ImportAPICollection with path: %s\n", filePath)

	// If filePath is empty, show file dialog
	if filePath == "" {
		fmt.Printf("Debug: Empty filePath, showing dialog\n")
		var err error
		filePath, err = a.BrowseForAPICollectionFile()
		if err != nil {
			fmt.Printf("Debug: BrowseForAPICollectionFile error: %v\n", err)
			return APICollection{}, fmt.Errorf("failed to browse for file: %w", err)
		}
		if filePath == "" {
			fmt.Printf("Debug: No file selected\n")
			return APICollection{}, fmt.Errorf("no file selected")
		}
	}

	fmt.Printf("Debug: Reading file: %s\n", filePath)

	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Debug: Failed to read file: %v\n", err)
		return APICollection{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Detect the collection type
	collectionType := detectCollectionType(data)
	fmt.Printf("Debug: Detected collection type: %s\n", collectionType)

	switch collectionType {
	case "postman":
		return a.importPostmanCollection(filePath, data)
	case "openapi2", "openapi3":
		return a.ImportOpenAPICollection(filePath)
	default:
		return APICollection{}, fmt.Errorf("unsupported file format. Please provide a valid OpenAPI specification or Postman collection")
	}
}

// importPostmanCollection imports a Postman collection
func (a *App) importPostmanCollection(filePath string, data []byte) (APICollection, error) {
	fmt.Printf("Debug: Importing Postman collection from: %s\n", filePath)

	// Parse the Postman collection
	collection, err := postman.ParseCollection(strings.NewReader(string(data)))
	if err != nil {
		fmt.Printf("Debug: Failed to parse Postman collection: %v\n", err)
		return APICollection{}, fmt.Errorf("failed to parse Postman collection: %w", err)
	}

	// Create our API collection
	apiCollection := APICollection{
		ID:          uuid.New().String(),
		Name:        collection.Info.Name,
		Description: collection.Info.Description.Content, // Fix: access Content field
		Version:     collection.Info.Version,
		FilePath:    filePath,
		Type:        "postman",
	}

	// Extract variables
	for _, variable := range collection.Variables {
		apiCollection.Variables = append(apiCollection.Variables, APIVariable{
			Name:        variable.Key,
			Value:       variable.Value,
			Description: variable.Description,
		})
	}

	// Extract authentication schemes
	if collection.Auth != nil {
		scheme := convertPostmanAuth(collection.Auth)
		if scheme != nil {
			apiCollection.SecuritySchemes = append(apiCollection.SecuritySchemes, *scheme)
			apiCollection.ActiveSecurity = scheme.ID
		}
	}

	// Extract requests from items
	var requests []APIRequest
	extractPostmanRequests(collection.Items, "", &requests, &apiCollection)
	apiCollection.Requests = requests

	fmt.Printf("Debug: Postman collection parsed successfully. Name: %s, Requests: %d\n",
		apiCollection.Name, len(apiCollection.Requests))

	// Save the collection to the project
	fmt.Printf("Debug: Saving collection to project\n")
	collectionID, err := a.SaveAPICollection(apiCollection)
	if err != nil {
		fmt.Printf("Debug: SaveAPICollection error: %v\n", err)
		return apiCollection, fmt.Errorf("failed to save imported collection: %w", err)
	}
	fmt.Printf("Debug: Collection saved with ID: %s\n", collectionID)

	// Set this as the selected API collection
	fmt.Printf("Debug: Setting as selected API collection\n")
	if err := a.SetSelectedAPICollection(apiCollection.ID); err != nil {
		fmt.Printf("Debug: Failed to set selected API collection: %v\n", err)
	}

	fmt.Printf("Debug: ImportPostmanCollection completed successfully\n")
	return apiCollection, nil
}

// convertPostmanAuth converts Postman authentication to our security scheme format
func convertPostmanAuth(auth *postman.Auth) *APISecurityScheme {
	if auth == nil {
		return nil
	}

	scheme := &APISecurityScheme{
		ID:   uuid.New().String(),
		Name: string(auth.Type),
		Type: string(auth.Type),
	}

	switch auth.Type {
	case postman.APIKey:
		scheme.Type = "apiKey"
		scheme.Name = "API Key"
		// Extract key and value from auth parameters
		for _, param := range auth.APIKey {
			if param.Key == "key" {
				if value, ok := param.Value.(string); ok {
					scheme.KeyName = value
				}
			} else if param.Key == "value" {
				if value, ok := param.Value.(string); ok {
					scheme.Value = value
				}
			} else if param.Key == "in" {
				if value, ok := param.Value.(string); ok {
					scheme.In = value
				}
			}
		}
	case postman.Basic:
		scheme.Type = "http"
		scheme.Scheme = "basic"
		scheme.Name = "Basic Authentication"
		// Extract username and password
		for _, param := range auth.Basic {
			if param.Key == "username" || param.Key == "password" {
				// For basic auth, we'll store the base64 encoded value
				scheme.Value = "dXNlcm5hbWU6cGFzc3dvcmQ=" // placeholder
			}
		}
	case postman.Bearer:
		scheme.Type = "http"
		scheme.Scheme = "bearer"
		scheme.Name = "Bearer Token"
		for _, param := range auth.Bearer {
			if param.Key == "token" {
				if value, ok := param.Value.(string); ok {
					scheme.Value = value
				}
			}
		}
	}

	return scheme
}

// extractPostmanRequests recursively extracts requests from Postman items
func extractPostmanRequests(items []*postman.Items, folderPath string, requests *[]APIRequest, collection *APICollection) {
	for _, item := range items {
		if item.Request != nil {
			// This is a request item
			request := convertPostmanRequest(item, folderPath)
			*requests = append(*requests, request)
		} else if len(item.Items) > 0 {
			// This is a folder item
			newFolderPath := item.Name
			if folderPath != "" {
				newFolderPath = folderPath + "/" + item.Name
			}
			extractPostmanRequests(item.Items, newFolderPath, requests, collection)
		}
	}
}

// convertPostmanRequest converts a Postman request to our API request format
func convertPostmanRequest(item *postman.Items, folderPath string) APIRequest {
	request := APIRequest{
		ID:          uuid.New().String(),
		Name:        item.Name,
		Description: item.Description,
		Folder:      folderPath,
		Headers:     []APIHeader{},    // Initialize as empty slice, not nil
		Parameters:  []APIParameter{}, // Initialize as empty slice, not nil
		Examples:    []APIExample{},   // Initialize as empty slice, not nil
	}

	if item.Request != nil {
		req := item.Request

		// Set method
		request.Method = string(req.Method) // Fix: Method is already a string

		// Set URL and path
		if req.URL != nil {
			if req.URL.Raw != "" {
				request.URL = req.URL.Raw
				// Extract path from URL
				if parsedURL, err := url.Parse(req.URL.Raw); err == nil {
					request.Path = parsedURL.Path
					request.Host = parsedURL.Host
				}

				// Check if there are additional query parameters in the URL components
				// that might not be in the Raw URL
				if len(req.URL.Query) > 0 {
					parsedURL, err := url.Parse(req.URL.Raw)
					if err == nil {
						existingQuery := parsedURL.Query()

						// Add any query parameters from the URL components that aren't already in the Raw URL
						for _, query := range req.URL.Query {
							if query.Key != "" && query.Value != "" {
								// Only add if not already present in the Raw URL
								if !existingQuery.Has(query.Key) {
									existingQuery.Add(query.Key, query.Value)
								}
							}
						}

						// Rebuild the URL with all query parameters
						parsedURL.RawQuery = existingQuery.Encode()
						request.URL = parsedURL.String()
					}
				}
			} else {
				// Build URL from components
				if len(req.URL.Host) > 0 {
					request.Host = strings.Join(req.URL.Host, ".")
				}
				if len(req.URL.Path) > 0 {
					request.Path = "/" + strings.Join(req.URL.Path, "/")
				}

				// Build query string from query parameters
				var queryParams []string
				for _, query := range req.URL.Query {
					if query.Key != "" && query.Value != "" {
						queryParams = append(queryParams, fmt.Sprintf("%s=%s",
							url.QueryEscape(query.Key), url.QueryEscape(query.Value)))
					}
				}

				// Construct the full URL
				baseURL := "https://" + request.Host + request.Path
				if len(queryParams) > 0 {
					request.URL = baseURL + "?" + strings.Join(queryParams, "&")
				} else {
					request.URL = baseURL
				}
			}
		}

		// Set headers
		// First, add the Host header if we have a host
		if request.Host != "" {
			request.Headers = append(request.Headers, APIHeader{
				Name:  "Host",
				Value: request.Host,
			})
		}

		// Then add headers from the Postman request
		for _, header := range req.Header {
			if header.Key != "" {
				// Skip Host header if it's already in the Postman headers to avoid duplicates
				if strings.ToLower(header.Key) == "host" && request.Host != "" {
					// Update the existing Host header with the value from Postman
					for i := range request.Headers {
						if strings.ToLower(request.Headers[i].Name) == "host" {
							request.Headers[i].Value = header.Value
							if header.Description != "" {
								request.Headers[i].Description = header.Description
							}
							break
						}
					}
					continue
				}

				apiHeader := APIHeader{
					Name:  header.Key,
					Value: header.Value,
				}

				// Include description if available
				if header.Description != "" {
					apiHeader.Description = header.Description
				}

				// Note: We don't include the Disabled field in our APIHeader struct
				// but we could skip disabled headers if needed
				if !header.Disabled {
					request.Headers = append(request.Headers, apiHeader)
				}
			}
		}

		// Set body
		if req.Body != nil {
			switch req.Body.Mode {
			case "raw":
				request.Body = req.Body.Raw
			case "formdata":
				// Convert form data to a readable format
				var formParts []string
				if formData, ok := req.Body.FormData.([]interface{}); ok {
					for _, data := range formData {
						if dataMap, ok := data.(map[string]interface{}); ok {
							if key, keyOk := dataMap["key"].(string); keyOk {
								if value, valueOk := dataMap["value"].(string); valueOk {
									formParts = append(formParts, fmt.Sprintf("%s=%s", key, value))
								}
							}
						}
					}
				}
				request.Body = strings.Join(formParts, "&")
			case "urlencoded":
				// Convert URL encoded data
				var urlParts []string
				if urlEncoded, ok := req.Body.URLEncoded.([]interface{}); ok {
					for _, data := range urlEncoded {
						if dataMap, ok := data.(map[string]interface{}); ok {
							if key, keyOk := dataMap["key"].(string); keyOk {
								if value, valueOk := dataMap["value"].(string); valueOk {
									urlParts = append(urlParts, fmt.Sprintf("%s=%s", key, value))
								}
							}
						}
					}
				}
				request.Body = strings.Join(urlParts, "&")
			}
		}

		// Extract query parameters
		if req.URL != nil {
			for _, query := range req.URL.Query {
				if query.Key != "" {
					param := APIParameter{
						Name:     query.Key,
						In:       "query",
						Required: false,
						Type:     "string",
					}

					// Include the value if available
					if query.Value != "" {
						param.Description = fmt.Sprintf("Default value: %s", query.Value)
					}

					request.Parameters = append(request.Parameters, param)
				}
			}
		}

		// Note: Examples should only contain actual response examples from Postman,
		// not the request itself. The request data is stored in the APIRequest fields above.
	}

	return request
}

// ImportAPICollectionAsync initiates an asynchronous import of an API collection (OpenAPI or Postman)
func (a *App) ImportAPICollectionAsync(filePath string) (string, error) {
	// Generate an import ID that will be used to track this import operation
	importID := uuid.New().String()

	// Start the import in a background goroutine
	go func() {
		collection, err := a.ImportAPICollection(filePath)
		if err != nil {
			// Emit an error event for the frontend
			rt.EventsEmit(a.ctx, "api-collection:import-error", map[string]interface{}{
				"importID": importID,
				"error":    err.Error(),
			})
			return
		}

		// Emit a success event with the collection data
		rt.EventsEmit(a.ctx, "api-collection:import-success", map[string]interface{}{
			"importID":   importID,
			"collection": collection,
		})
	}()

	return importID, nil
}
