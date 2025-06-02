package network

import (
	"strconv"
	"strings"
)

// HTTPRequest represents the request part of an HTTP transaction
type HTTPRequest struct {
	Host string `json:"host"`
	TLS  bool   `json:"tls"`  // Whether the request is using TLS/HTTPS
	Dump string `json:"dump"` // Raw HTTP request dump
}

func (r *HTTPRequest) Headers() map[string]string {
	headers := make(map[string]string)
	headerLines := strings.Split(r.Dump, "\r\n\r\n")[0]
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
	// fmt.Println("Method:", method)
	return strings.ToUpper(method)
}

func (r *HTTPRequest) URL() string {
	if r.TLS {
		return "https://" + r.Host + strings.Split(r.Dump, " ")[1]
	}
	return "http://" + r.Host + strings.Split(r.Dump, " ")[1]
}

// HTTPResponse represents the response part of an HTTP transaction
type HTTPResponse struct {
	Dump string `json:"dump"` // Raw HTTP response dump
}

// Printable returns the printable version of the HTTP response
func (r *HTTPResponse) Printable() string {
	printable := GetPrintableResponse([]byte(r.Dump))
	// fmt.Println("Printable:", printable)
	return printable
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
	HistoryID            string        `json:"-"`                    // ID of the corresponding transaction in proxy history
	// Flag to determine if response should be intercepted or auto-forwarded
	InterceptResponse bool `json:"interceptResponse"` // Whether to intercept the response or auto-forward it
	// Fields for storing the actual response data to send back to browser
	ActualResponseStatus     string            `json:"-"` // The actual HTTP status to send to browser
	ActualResponseStatusCode int               `json:"-"` // The actual HTTP status code to send to browser
	ActualResponseHeaders    map[string]string `json:"-"` // The actual headers to send to browser
	ActualResponseBody       []byte            `json:"-"` // The actual response body to send to browser
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
