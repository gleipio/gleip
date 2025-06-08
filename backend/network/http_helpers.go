package network

// HTTPHelper provides utility functions for working with HTTP objects in the frontend
type HTTPHelper struct{}

// NewHTTPHelper creates a new HTTPHelper instance
func NewHTTPHelper() *HTTPHelper {
	return &HTTPHelper{}
}

// Request helper methods

// GetRequestMethod extracts the HTTP method from an HTTPRequest
func (h *HTTPHelper) GetRequestMethod(request HTTPRequest) string {
	return request.Method()
}

// GetRequestURL extracts the full URL from an HTTPRequest
func (h *HTTPHelper) GetRequestURL(request HTTPRequest) string {
	return request.URL()
}

// GetRequestHeaders extracts headers from an HTTPRequest
func (h *HTTPHelper) GetRequestHeaders(request HTTPRequest) map[string]string {
	return request.Headers()
}

// GetRequestBody extracts the body from an HTTPRequest
func (h *HTTPHelper) GetRequestBody(request HTTPRequest) string {
	return string(request.Body())
}

// GetRequestHost extracts the host from an HTTPRequest
func (h *HTTPHelper) GetRequestHost(request HTTPRequest) string {
	return request.Host
}

// GetRequestTLS returns whether the request uses TLS
func (h *HTTPHelper) GetRequestTLS(request HTTPRequest) bool {
	return request.TLS
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

// FormatRequestSummary creates a summary string for a request
func (h *HTTPHelper) FormatRequestSummary(request HTTPRequest) string {
	return request.Method() + " " + request.URL()
}

// FormatResponseSummary creates a summary string for a response
func (h *HTTPHelper) FormatResponseSummary(response HTTPResponse) string {
	return response.Status()
}
