package network

import (
	"fmt"
	"net/http"
	"strings"
)

// DefaultResponseFormatter implements the ResponseFormatter interface
type DefaultResponseFormatter struct{}

// NewResponseFormatter creates a new response formatter
func NewResponseFormatter() ResponseFormatter {
	return &DefaultResponseFormatter{}
}

// FormatRequest formats the request into a readable string
func (f *DefaultResponseFormatter) FormatRequest(req *http.Request, body string) string {
	var dump strings.Builder

	// Write request line
	dump.WriteString(fmt.Sprintf("%s %s HTTP/1.1\r\n", req.Method, req.URL.Path))

	// Write headers
	for key, values := range req.Header {
		for _, value := range values {
			dump.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	// Write body if present
	if body != "" {
		dump.WriteString("\r\n")
		dump.WriteString(body)
	}

	return dump.String()
}

// FormatResponse formats the response into a readable string
func (f *DefaultResponseFormatter) FormatResponse(resp *http.Response, body []byte) string {
	var dump strings.Builder

	// Write status line
	dump.WriteString(fmt.Sprintf("HTTP/1.1 %s\r\n", resp.Status))

	// Write headers
	for key, values := range resp.Header {
		for _, value := range values {
			dump.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
		}
	}

	// Write body if present
	if len(body) > 0 {
		dump.WriteString("\r\n")
		dump.WriteString(string(body))
	}

	return dump.String()
}
