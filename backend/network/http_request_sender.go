package network

import (
	"Gleip/backend/network/http_utils"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DefaultRequestSender implements the RequestSender interface
type DefaultRequestSender struct {
	httpClient           http_utils.HTTPClient
	responseDecompressor ResponseDecompressor
	responseFormatter    ResponseFormatter
}

// NewRequestSender creates a new request sender with dependencies
func NewRequestSender(client http_utils.HTTPClient, decompressor ResponseDecompressor, formatter ResponseFormatter) RequestSender {
	return &DefaultRequestSender{
		httpClient:           client,
		responseDecompressor: decompressor,
		responseFormatter:    formatter,
	}
}

// SendRequest sends an HTTP request and returns the response
func (s *DefaultRequestSender) SendRequest(method, url, host, body string, headers map[string]string, gunzipResponse bool, tls bool) (*HTTPTransaction, error) {
	// If host is provided, use it to construct the full URL
	if host != "" {
		url = s.constructURL(url, host, tls)
	}

	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set all headers exactly as provided by the user
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decompress response if needed and update headers
	if gunzipResponse {
		contentEncoding := resp.Header.Get("Content-Encoding")
		if contentEncoding != "" {
			if decompressedBody, err := s.responseDecompressor.Decompress(respBody, contentEncoding); err == nil {
				respBody = decompressedBody
				// Remove Content-Encoding header since we decompressed
				resp.Header.Del("Content-Encoding")
				// Update Content-Length header to reflect new body size
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(respBody)))
			}
		}
	}

	// Create transaction
	formattedResponseDump := s.responseFormatter.FormatResponse(resp, respBody)
	transaction := &HTTPTransaction{
		ID: uuid.New().String(),
		Request: HTTPRequest{
			Host: host,
			Dump: s.responseFormatter.FormatRequest(req, body),
			TLS:  tls,
		},
		Response: &HTTPResponse{
			Dump: formattedResponseDump,
		},
		Timestamp: time.Now().Format(time.RFC3339),
		SeqNumber: 0,
		CameFrom:  "gleipflow",
	}

	return transaction, nil
}

// SendRawRequest sends an HTTP request using raw request text
func (s *DefaultRequestSender) SendRawRequest(request HTTPRequest, gunzipResponse bool) (*HTTPTransaction, error) {
	return s.sendRawRequestInternal(request, gunzipResponse, nil)
}

// SendRawRequestWithTimeout sends an HTTP request with a timeout
func (s *DefaultRequestSender) SendRawRequestWithTimeout(request HTTPRequest, gunzipResponse bool, timeout time.Duration) (*HTTPTransaction, error) {
	return s.sendRawRequestInternal(request, gunzipResponse, &timeout)
}

// constructURL constructs a full URL from parts
func (s *DefaultRequestSender) constructURL(urlPath, host string, tls bool) string {
	// Remove any protocol from the URL if present
	if strings.HasPrefix(urlPath, "http://") || strings.HasPrefix(urlPath, "https://") {
		parts := strings.SplitN(urlPath, "://", 2)
		if len(parts) > 1 {
			urlPath = parts[1]
		}
	}
	// Remove any host from the URL path
	if strings.Contains(urlPath, "/") {
		parts := strings.SplitN(urlPath, "/", 2)
		urlPath = "/" + parts[1]
	}
	// Construct new URL with the specified host using TLS setting
	protocol := "http"
	if tls {
		protocol = "https"
	}
	return protocol + "://" + host + urlPath
}

// parseRawHTTPRequest parses a raw HTTP request string and extracts components
func (s *DefaultRequestSender) parseRawHTTPRequest(rawRequest, host string, tls bool) (method, finalURL, body string, headers map[string]string, isHTTP2 bool, err error) {
	// Normalize line endings
	normalizedRequest := strings.ReplaceAll(rawRequest, "\r\n", "\n")
	normalizedRequest = strings.ReplaceAll(normalizedRequest, "\n", "\r\n")

	// Split the request into header and body sections
	parts := strings.SplitN(normalizedRequest, "\r\n\r\n", 2)

	// Extract headers section
	headersSection := parts[0]
	headerLines := strings.Split(headersSection, "\r\n")

	// Extract body if it exists
	if len(parts) > 1 {
		body = parts[1]
	}

	// Parse headers first to potentially extract host
	headers = make(map[string]string)
	var hostFromHeader string
	for i := 1; i < len(headerLines); i++ {
		line := headerLines[i]
		if line == "" {
			break
		}

		colonIndex := strings.Index(line, ":")
		if colonIndex > 0 {
			key := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])
			headers[key] = value

			// Extract host from Host header if host parameter is empty
			if host == "" && strings.ToLower(key) == "host" {
				hostFromHeader = value
			}
		}
	}

	// Use host from header if host parameter is empty
	if host == "" && hostFromHeader != "" {
		host = hostFromHeader
	}

	// Parse the request line
	if len(headerLines) > 0 {
		requestLine := headerLines[0]
		requestParts := strings.Fields(requestLine)
		if len(requestParts) >= 3 {
			method = requestParts[0]
			actualPath := requestParts[1]
			isHTTP2 = strings.Contains(requestParts[2], "HTTP/2")

			// Construct the full URL
			if host != "" {
				protocol := "http"
				if tls {
					protocol = "https"
				}

				// Handle paths that might be full URLs
				if strings.HasPrefix(actualPath, "http://") || strings.HasPrefix(actualPath, "https://") {
					if parsedURL, err := neturl.Parse(actualPath); err == nil {
						actualPath = parsedURL.Path
						if parsedURL.RawQuery != "" {
							actualPath += "?" + parsedURL.RawQuery
						}
					}
				}

				finalURL = protocol + "://" + host + actualPath
			} else {
				// If still no host, we can't construct a valid URL
				return "", "", "", nil, false, fmt.Errorf("no host found in request - cannot construct URL. Please ensure Host header is present in the raw request")
			}
		} else {
			return "", "", "", nil, false, fmt.Errorf("invalid request line: %s", requestLine)
		}
	} else {
		return "", "", "", nil, false, fmt.Errorf("no request line found")
	}

	return method, finalURL, body, headers, isHTTP2, nil
}

// createHTTPTransport creates a standardized HTTP transport with HTTP/2 support
func CreateHTTPTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		// Enable HTTP/2
		ForceAttemptHTTP2: true,
		// Close connections after use
		DisableKeepAlives: true,
		// Set timeouts
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
}

// sendRawRequestInternal is the generalized internal method for sending raw requests
func (s *DefaultRequestSender) sendRawRequestInternal(request HTTPRequest, gunzipResponse bool, timeout *time.Duration) (*HTTPTransaction, error) {
	// Parse the raw request
	actualMethod, finalURL, body, headers, _, err := s.parseRawHTTPRequest(request.Dump, request.Host, request.TLS)
	if err != nil {
		return nil, err
	}

	// Extract the actual host used (it might have been extracted from the Host header)
	extractedHost := request.Host
	if extractedHost == "" {
		// Try to extract host from the finalURL
		if finalURL != "" {
			if parsedURL, err := neturl.Parse(finalURL); err == nil {
				extractedHost = parsedURL.Host
			}
		}
	}

	// Create the request with or without context based on timeout
	var req *http.Request
	if timeout != nil {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		req, err = http.NewRequestWithContext(ctx, actualMethod, finalURL, strings.NewReader(body))
	} else {
		req, err = http.NewRequest(actualMethod, finalURL, strings.NewReader(body))
	}

	if err != nil {
		return nil, err
	}

	// Set headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Send the request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Decompress response if needed and update headers
	if gunzipResponse {
		contentEncoding := resp.Header.Get("Content-Encoding")
		if contentEncoding != "" {
			if decompressedBody, err := s.responseDecompressor.Decompress(respBody, contentEncoding); err == nil {
				respBody = decompressedBody
				// Remove Content-Encoding header since we decompressed
				resp.Header.Del("Content-Encoding")
				// Update Content-Length header to reflect new body size
				resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(respBody)))
			}
		}
	}

	// Create transaction
	formattedResponseDump := s.responseFormatter.FormatResponse(resp, respBody)
	transaction := &HTTPTransaction{
		ID: uuid.New().String(),
		Request: HTTPRequest{
			Host: extractedHost,
			Dump: request.Dump,
			TLS:  request.TLS,
		},
		Response: &HTTPResponse{
			Dump: formattedResponseDump,
		},
		Timestamp: time.Now().Format(time.RFC3339),
		SeqNumber: 0,
		CameFrom:  "gleipflow",
	}

	return transaction, nil
}
