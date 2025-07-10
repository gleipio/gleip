package network

import (
	"Gleip/backend/network/http_utils"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPTransaction represents a complete HTTP transaction (request + optional response)
type HTTPTransaction struct {
	ID                   string        `json:"id"`
	Request              HTTPRequest   `json:"request"`
	Response             *HTTPResponse `json:"response,omitempty"`
	Timestamp            string        `json:"timestamp"`
	Done                 chan struct{} `json:"-"`                    // Used for interception gleip control
	SeqNumber            int           `json:"seqNumber"`            // Sequence number for the request
	WaitingForResponse   bool          `json:"waitingForResponse"`   // Whether this request is waiting for response interception
	InterceptedRequestID string        `json:"interceptedRequestId"` // ID of the original intercepted request for response matching
	HistoryID            string        `json:"historyId"`            // ID of the transaction in history for updates
	InterceptResponse    bool          `json:"interceptResponse"`    // Whether to intercept the response
	WasDropped           bool          `json:"wasDropped"`           // Whether the request was dropped by the user
	CameFrom             string        `json:"cameFrom"`             // Source of the transaction: "browser", "intercept", "gleipflow"
	AutoForwarded        bool          `json:"autoForwarded"`        // Whether the request was auto-forwarded and should be hidden from UI
}

// HTTPTransactionSummary represents the summary data for the list view
type HTTPTransactionSummary struct {
	ID           string  `json:"id"`
	Timestamp    string  `json:"timestamp"`
	Method       string  `json:"method"`
	URL          string  `json:"url"`
	StatusCode   *int    `json:"statusCode,omitempty"` // Pointer to handle cases with no response
	Status       *string `json:"status,omitempty"`     // Pointer to handle cases with no response
	ResponseSize int     `json:"responseSize"`         // Size of the response in bytes
	SeqNumber    int     `json:"seqNumber"`            // Sequential request number (1-based index)
}

// HTTPTransactionChunk represents a chunk of request or response data
type HTTPTransactionChunk struct {
	TransactionID string `json:"transactionId"`
	Type          string `json:"type"`        // "request" or "response"
	ChunkIndex    int    `json:"chunkIndex"`  // 0-based chunk index
	ChunkData     string `json:"chunkData"`   // The actual chunk content
	TotalChunks   int    `json:"totalChunks"` // Total number of chunks for this data
	IsComplete    bool   `json:"isComplete"`  // Whether this is the last chunk
	TotalSize     int    `json:"totalSize"`   // Total size of the complete data
}

// HTTPRequest represents the request part of an HTTP transaction
type HTTPRequest struct {
	Host string `json:"host"`
	TLS  bool   `json:"tls"`  // Whether the request is using TLS/HTTPS
	Dump string `json:"dump"` // A string is the set of all strings of 8-bit bytes not necessarily representing UTF-8-encoded text
}

// HTTPResponse represents the response part of an HTTP transaction
type HTTPResponse struct {
	Dump string `json:"dump"` // A string is the set of all strings of 8-bit bytes not necessarily representing UTF-8-encoded text
}

// RequestLike interface defines the common structure for HTTP request objects
// Both HTTPRequest and PhantomRequest implement this interface
type RequestLike interface {
	GetHost() string
	GetTLS() bool
	GetDump() string
	GetDumpBytes() []byte
}

// RequestSender interface for sending HTTP requests (Single Responsibility)
type RequestSender interface {
	SendRequest(method, url, host, body string, headers map[string]string, gunzipResponse bool, tls bool) (*HTTPTransaction, error)
	SendRawRequest(request HTTPRequest, gunzipResponse bool) (*HTTPTransaction, error)
	SendRawRequestWithTimeout(request HTTPRequest, gunzipResponse bool, timeout time.Duration) (*HTTPTransaction, error)
}

// ResponseFormatter interface for formatting responses (Single Responsibility)
type ResponseFormatter interface {
	FormatRequest(req *http.Request, body string) string
	FormatResponse(resp *http.Response, body []byte) string
}

// ResponseDecompressor interface for decompressing responses (Single Responsibility)
type ResponseDecompressor interface {
	Decompress(body []byte, contentEncoding string) ([]byte, error)
}

// Implement RequestLike interface
func (r *HTTPRequest) GetHost() string      { return r.Host }
func (r *HTTPRequest) GetTLS() bool         { return r.TLS }
func (r *HTTPRequest) GetDump() string      { return r.Dump }
func (r *HTTPRequest) GetDumpBytes() []byte { return []byte(r.Dump) }

func (r *HTTPResponse) GetDumpBytes() []byte { return []byte(r.Dump) }

func (r *HTTPRequest) Headers() map[string]string {
	headers := make(map[string]string)
	headerLines := strings.Split(r.GetDump(), "\r\n\r\n")[0]
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

func (r *HTTPRequest) Body() []byte {
	body := strings.Split(r.Dump, "\r\n\r\n")
	if len(body) > 1 {
		return []byte(body[1])
	}
	return []byte{}
}

func (r *HTTPRequest) Method() string {
	method := strings.Split(r.Dump, " ")[0]
	return strings.ToUpper(method)
}

func (r *HTTPRequest) URL() string {
	urlPart := strings.Split(r.Dump, " ")[1]

	// If the URL part already contains a full URL (starts with http:// or https://), use it as-is
	if strings.HasPrefix(urlPart, "http://") || strings.HasPrefix(urlPart, "https://") {
		return urlPart
	}

	// Otherwise, construct the full URL from protocol + host + path
	if r.TLS {
		return "https://" + r.Host + urlPart
	}
	return "http://" + r.Host + urlPart
}

// Printable returns the printable version of the HTTP response
func (r *HTTPResponse) Printable() string {
	// Check if this response has already been decompressed by looking for Content-Encoding
	dump := r.Dump
	hasContentEncoding := strings.Contains(dump, "Content-Encoding:")

	// If it has Content-Encoding (gzip/brotli), it needs decompression for display
	if hasContentEncoding {
		return http_utils.GetPrintableResponse([]byte(dump))
	}

	// If no Content-Encoding, check for Transfer-Encoding: chunked
	hasTransferEncoding := strings.Contains(dump, "Transfer-Encoding: chunked")
	if hasTransferEncoding {
		return http_utils.GetPrintableResponse([]byte(dump))
	}

	// Otherwise, it's already processed
	return dump
}

func (r *HTTPResponse) Status() string {
	return strings.Split(r.Dump, " ")[2]
}

func (r *HTTPResponse) StatusCode() int {
	statusCode, err := strconv.Atoi(strings.Split(r.Dump, " ")[1])
	if err != nil {
		return 0
	}
	return statusCode
}

// Headers extracts headers from the response dump
func (r *HTTPResponse) Headers() map[string]string {
	headers := make(map[string]string)

	// Split response into header and body sections
	parts := strings.SplitN(r.Dump, "\r\n\r\n", 2)
	if len(parts) == 0 {
		return headers
	}

	headerSection := parts[0]
	headerLines := strings.Split(headerSection, "\r\n")

	// Skip the status line and parse headers
	for i := 1; i < len(headerLines); i++ {
		line := headerLines[i]
		if colonIndex := strings.Index(line, ":"); colonIndex > 0 {
			key := strings.TrimSpace(line[:colonIndex])
			value := strings.TrimSpace(line[colonIndex+1:])
			headers[key] = value
		}
	}

	return headers
}

// Body extracts the body from the response dump
func (r *HTTPResponse) Body() []byte {
	parts := strings.SplitN(r.Dump, "\r\n\r\n", 2)
	if len(parts) < 2 {
		return []byte{}
	}
	return []byte(parts[1])
}
