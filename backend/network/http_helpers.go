package network

import (
	"strings"
)

// HTTPHelper provides utility functions for working with HTTP objects in the frontend
//
// This helper uses the RequestLike interface to work with both HTTPRequest and PhantomRequest
// objects seamlessly. This allows:
// 1. Code reuse - same helper methods work for both types
// 2. Future extensibility - new request-like types can implement RequestLike
// 3. Frontend simplicity - frontend code doesn't need to know which type it's working with
type HTTPHelper struct{}

// NewHTTPHelper creates a new HTTPHelper instance
func NewHTTPHelper() *HTTPHelper {
	return &HTTPHelper{}
}

// parseRequestFromInterface extracts request data from any interface{} (handles Wails serialization)
func parseRequestFromInterface(request interface{}) (host string, tls bool, dump string) {
	// Handle direct struct types (HTTPRequest, PhantomRequest)
	if req, ok := request.(RequestLike); ok {
		return req.GetHost(), req.GetTLS(), req.GetDump()
	}

	// Handle Wails-serialized objects (map[string]interface{})
	if reqMap, ok := request.(map[string]interface{}); ok {
		host, _ = reqMap["host"].(string)
		tls, _ = reqMap["tls"].(bool)
		dump, _ = reqMap["dump"].(string)
		return
	}

	return "", false, ""
}

// parseHTTPMethod extracts HTTP method from dump
func parseHTTPMethod(dump string) string {
	if dump == "" {
		return ""
	}
	parts := strings.Split(dump, " ")
	if len(parts) > 0 {
		return strings.ToUpper(parts[0])
	}
	return ""
}

// parseHTTPURL constructs full URL from host, tls, and dump
func parseHTTPURL(host string, tls bool, dump string) string {
	if dump == "" || host == "" {
		return ""
	}

	parts := strings.Split(dump, " ")
	if len(parts) < 2 {
		return ""
	}

	path := parts[1]
	protocol := "http"
	if tls {
		protocol = "https"
	}

	return protocol + "://" + host + path
}

// parseHTTPPath extracts path from dump
func parseHTTPPath(dump string) string {
	if dump == "" {
		return ""
	}
	return strings.Split(dump, " ")[1]
}

// parseHTTPHeaders extracts headers from dump
func parseHTTPHeaders(dump string) map[string]string {
	headers := make(map[string]string)
	if dump == "" {
		return headers
	}

	headerLines := strings.Split(dump, "\r\n\r\n")[0]
	for _, line := range strings.Split(headerLines, "\r\n") {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
	}
	return headers
}

// parseHTTPBody extracts body from dump
func parseHTTPBody(dump string) string {
	if dump == "" {
		return ""
	}

	body := strings.Split(dump, "\r\n\r\n")
	if len(body) > 1 {
		return body[1]
	}
	return ""
}

// Request helper methods - work with both HTTPRequest and PhantomRequest

// GetRequestMethod extracts the HTTP method from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestMethod(request interface{}) string {
	_, _, dump := parseRequestFromInterface(request)
	return parseHTTPMethod(dump)
}

// GetRequestURL extracts the full URL from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestURL(request interface{}) string {
	host, tls, dump := parseRequestFromInterface(request)
	return parseHTTPURL(host, tls, dump)
}

// GetRequestPath extracts the path from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestPath(request interface{}) string {
	_, _, dump := parseRequestFromInterface(request)
	return parseHTTPPath(dump)
}

// GetRequestHeaders extracts headers from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestHeaders(request interface{}) map[string]string {
	_, _, dump := parseRequestFromInterface(request)
	return parseHTTPHeaders(dump)
}

// GetRequestBody extracts the body from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestBody(request interface{}) string {
	_, _, dump := parseRequestFromInterface(request)
	return parseHTTPBody(dump)
}

// GetRequestHost extracts the host from an HTTPRequest or PhantomRequest
func (h *HTTPHelper) GetRequestHost(request interface{}) string {
	host, _, _ := parseRequestFromInterface(request)
	return host
}

// GetRequestTLS returns whether the request uses TLS
func (h *HTTPHelper) GetRequestTLS(request interface{}) bool {
	_, tls, _ := parseRequestFromInterface(request)
	return tls
}

// Response helper methods

// GetResponseStatus extracts the status from an HTTPResponse
func (h *HTTPHelper) GetResponseStatus(response HTTPResponse) string {
	return response.Status()
}

// GetResponseStatusCode extracts the status code from an HTTPResponse
func (h *HTTPHelper) GetResponseStatusCode(response HTTPResponse) int {
	return response.StatusCode()
}

// GetResponsePrintable extracts the printable content from an HTTPResponse
func (h *HTTPHelper) GetResponsePrintable(response HTTPResponse) string {
	return response.Printable()
}

// GetResponseHeaders extracts headers from an HTTPResponse (if we add this method later)
func (h *HTTPHelper) GetResponseHeaders(response HTTPResponse) map[string]string {
	// TODO: Implement response headers extraction if needed
	return make(map[string]string)
}

// Utility methods

// FormatRequestSummary creates a summary string for any request-like object
func (h *HTTPHelper) FormatRequestSummary(request interface{}) string {
	host, tls, dump := parseRequestFromInterface(request)
	method := parseHTTPMethod(dump)
	url := parseHTTPURL(host, tls, dump)
	return method + " " + url
}

// FormatResponseSummary creates a summary string for a response
func (h *HTTPHelper) FormatResponseSummary(response HTTPResponse) string {
	return response.Status()
}
