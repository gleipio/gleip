package backend

import (
	"Gleip/backend/cert"
	"Gleip/backend/network"
	"Gleip/backend/network/http_utils"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Chunk size for loading large requests/responses (64KB chunks)
const CHUNK_SIZE = 64 * 1024

// ProxyServer represents the HTTP proxy server
// Now follows SOLID principles with dependency injection
type ProxyServer struct {
	ctx              context.Context
	transactionStore network.TransactionStore
	interceptQueue   InterceptQueue
	eventEmitter     TransactionEventEmitter
	server           *http.Server
	listener         net.Listener
	port             int
	isRunning        bool
	interceptMutex   sync.RWMutex
	interceptEnabled bool
	certManager      *cert.CertificateManager
	// Map to track intercepted requests by their request signature for response matching
	interceptedRequestMap map[string]string // key: "method:url", value: intercepted request ID
	// Map to track forwarded requests to avoid double interception
	forwardedRequests map[string]bool // key: "method:url:timestamp", value: true
}

// NewProxyServer creates a new proxy server instance with dependency injection
func NewProxyServer(ctx context.Context, port int, certManager *cert.CertificateManager, store network.TransactionStore, queue InterceptQueue, emitter TransactionEventEmitter) *ProxyServer {
	return &ProxyServer{
		ctx:                   ctx,
		port:                  port,
		transactionStore:      store,
		interceptQueue:        queue,
		eventEmitter:          emitter,
		interceptEnabled:      false,
		certManager:           certManager,
		interceptedRequestMap: make(map[string]string),
		forwardedRequests:     make(map[string]bool),
	}
}

// ResetRequestCounter resets the proxy's request counter to 0
func (p *ProxyServer) ResetRequestCounter() {
	if resettable, ok := p.transactionStore.(*network.InMemoryTransactionStore); ok {
		resettable.Reset()
	}
}

// SetRequestCounter sets the proxy's request counter to a specific value
func (p *ProxyServer) SetRequestCounter(value int) {
	if settable, ok := p.transactionStore.(*network.InMemoryTransactionStore); ok {
		settable.SetCounter(value)
	}
}

// Start starts the proxy server
func (p *ProxyServer) Start() error {
	if p.isRunning {
		return fmt.Errorf("proxy already running")
	}

	// Create listener on all interfaces
	addr := fmt.Sprintf("0.0.0.0:%d", p.port)
	fmt.Printf("Starting proxy server on %s\n", addr)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %v", err)
	}
	p.listener = listener

	// Create server
	p.server = &http.Server{
		Handler: http.HandlerFunc(p.handleProxy),
	}

	// Mark as running before starting server
	p.isRunning = true

	// Start server in a goroutine
	go func() {
		if err := p.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Proxy server error: %v\n", err)
			p.isRunning = false
		}
	}()

	fmt.Printf("Proxy server started successfully\n")
	return nil
}

// Stop stops the proxy server
func (p *ProxyServer) Stop() error {
	if !p.isRunning {
		return nil
	}

	if err := p.server.Close(); err != nil {
		return err
	}

	p.isRunning = false
	return nil
}

// GetRequests returns summaries of all intercepted requests
func (p *ProxyServer) GetRequests() []network.HTTPTransactionSummary {
	return p.transactionStore.GetSummaries()
}

// GetRequestsAfter returns summaries of intercepted requests after the given ID
func (p *ProxyServer) GetRequestsAfter(lastID string) []network.HTTPTransactionSummary {
	return p.transactionStore.GetSummariesAfter(lastID)
}

// GetTransactionDetails returns the full details for a specific transaction ID
func (p *ProxyServer) GetTransactionDetails(id string) (*network.HTTPTransaction, error) {
	return p.transactionStore.GetByID(id)
}

// GetTransactionChunk returns a chunk of request or response data for a specific transaction
func (p *ProxyServer) GetTransactionChunk(id string, dataType string, chunkIndex int) (*network.HTTPTransactionChunk, error) {
	transaction, err := p.transactionStore.GetByID(id)
	if err != nil {
		return nil, err
	}

	var data string
	var totalSize int

	// Get the appropriate data based on type
	switch dataType {
	case "request":
		if transaction.Request.Dump == "" {
			return nil, fmt.Errorf("request data not available")
		}
		data = transaction.Request.Dump
		totalSize = len(data)
	case "response":
		if transaction.Response == nil || transaction.Response.Printable() == "" {
			return nil, fmt.Errorf("response data not available")
		}
		data = transaction.Response.Printable()
		totalSize = len(data)
	default:
		return nil, fmt.Errorf("invalid data type: %s", dataType)
	}

	// Calculate chunk boundaries
	totalChunks := (totalSize + CHUNK_SIZE - 1) / CHUNK_SIZE // Ceiling division
	if chunkIndex < 0 || chunkIndex >= totalChunks {
		return nil, fmt.Errorf("chunk index out of range: %d", chunkIndex)
	}

	start := chunkIndex * CHUNK_SIZE
	end := start + CHUNK_SIZE
	if end > totalSize {
		end = totalSize
	}

	chunkData := data[start:end]
	isComplete := chunkIndex == totalChunks-1

	return &network.HTTPTransactionChunk{
		TransactionID: id,
		Type:          dataType,
		ChunkIndex:    chunkIndex,
		ChunkData:     chunkData,
		TotalChunks:   totalChunks,
		IsComplete:    isComplete,
		TotalSize:     totalSize,
	}, nil
}

// GetTransactionMetadata returns metadata about a transaction's data sizes
func (p *ProxyServer) GetTransactionMetadata(id string) (map[string]interface{}, error) {
	transaction, err := p.transactionStore.GetByID(id)
	if err != nil {
		return nil, err
	}

	metadata := map[string]interface{}{
		"id":          transaction.ID,
		"hasRequest":  transaction.Request.Dump != "",
		"hasResponse": transaction.Response != nil && transaction.Response.Printable() != "",
	}

	if transaction.Request.Dump != "" {
		requestSize := len(transaction.Request.Dump)
		metadata["requestSize"] = requestSize
		metadata["requestChunks"] = (requestSize + CHUNK_SIZE - 1) / CHUNK_SIZE
	}

	if transaction.Response != nil && transaction.Response.Printable() != "" {
		responseSize := len(transaction.Response.Printable())
		metadata["responseSize"] = responseSize
		metadata["responseChunks"] = (responseSize + CHUNK_SIZE - 1) / CHUNK_SIZE
	}

	return metadata, nil
}

// GetInterceptedRequests returns all requests waiting for interception
func (p *ProxyServer) GetInterceptedRequests() []*network.HTTPTransaction {
	p.interceptMutex.RLock()
	defer p.interceptMutex.RUnlock()

	var filteredRequests []*network.HTTPTransaction

	allRequests := p.interceptQueue.GetAll()
	if allRequests == nil {
		return []*network.HTTPTransaction{}
	}

	for _, req := range allRequests {
		// Only include requests that are not auto-forwarded
		if req != nil && !req.AutoForwarded {
			filteredRequests = append(filteredRequests, req)
		}
	}

	return filteredRequests
}

// SetInterceptEnabled enables or disables request interception
func (p *ProxyServer) SetInterceptEnabled(enabled bool) {
	p.interceptMutex.Lock()
	defer p.interceptMutex.Unlock()
	p.interceptEnabled = enabled

	// If disabling interception, forward all pending requests
	if !enabled {
		p.forwardAllInterceptedRequests()
	}
}

// GetInterceptEnabled returns the current interception status
func (p *ProxyServer) GetInterceptEnabled() bool {
	p.interceptMutex.RLock()
	defer p.interceptMutex.RUnlock()
	return p.interceptEnabled
}

// forwardAllInterceptedRequests forwards all pending intercepted requests without modification
func (p *ProxyServer) forwardAllInterceptedRequests() {
	// This function assumes the interceptMutex is already locked by the caller
	for _, req := range p.interceptQueue.GetAll() {
		// Remove the intercepted request from history since it will be re-added by the normal proxy flow
		if req.HistoryID != "" {
			// We can't directly remove from the store, so we'll handle this differently
			// The transaction will be updated when it's forwarded
		}

		// Signal that the request can continue without modification
		close(req.Done)
		p.interceptQueue.Remove(req.ID)
	}
}

// ModifyInterceptedRequest modifies an intercepted request and allows it to continue without making a new HTTP request
func (p *ProxyServer) ModifyInterceptedRequest(id string, method, host string, headers map[string]string, rawRequest string) error {
	p.interceptMutex.Lock()
	defer p.interceptMutex.Unlock()

	req, exists := p.interceptQueue.Get(id)
	if !exists {
		return fmt.Errorf("request not found")
	}

	// Modify the request details for display purposes
	req.Request.Host = host
	req.Request.Dump = rawRequest

	// Update the corresponding history transaction with the modified request
	if req.HistoryID != "" {
		// Create the update transaction with proper request data
		updateTx := *req
		updateTx.ID = req.HistoryID // Use the original history ID

		// Update the transaction in the store
		err := p.transactionStore.Update(updateTx)
		if err != nil {
			fmt.Printf("Error updating transaction in store: %v\n", err)
		} else {
			fmt.Printf("Debug: Updated history transaction %s with modified request\n", updateTx.ID)
		}

		// Emit event for updated transaction
		if p.eventEmitter != nil {
			p.eventEmitter.EmitTransactionUpdate(updateTx)
		}
	}

	// Signal that the request can continue with the original flow
	close(req.Done)
	p.interceptQueue.Remove(id)
	return nil
}

// ModifyInterceptedResponse modifies an intercepted response and allows it to continue
func (p *ProxyServer) ModifyInterceptedResponse(id string, rawResponseDump string) error {
	p.interceptMutex.Lock()
	defer p.interceptMutex.Unlock()

	req, exists := p.interceptQueue.Get(id)
	if !exists {
		return fmt.Errorf("request not found")
	}

	if req.Response == nil {
		return fmt.Errorf("response not available yet")
	}

	// Modify the response for display
	req.Response.Dump = rawResponseDump

	// Clean up the request signature mapping
	p.cleanupRequestMapping(req)

	// Update the corresponding history transaction with the modified response
	if req.HistoryID != "" {
		// Create the update transaction with proper response data
		updateTx := *req
		updateTx.ID = req.HistoryID // Use the original history ID

		// Ensure the response is properly set for history
		if updateTx.Response == nil {
			updateTx.Response = &network.HTTPResponse{}
		}
		updateTx.Response.Dump = rawResponseDump

		// Update the transaction in the store
		err := p.transactionStore.Update(updateTx)
		if err != nil {
			fmt.Printf("Error updating transaction in store: %v\n", err)
		} else {
			fmt.Printf("Debug: Updated history transaction %s with response %d\n", updateTx.ID, updateTx.Response.StatusCode())
		}

		// Emit event for updated transaction
		if p.eventEmitter != nil {
			p.eventEmitter.EmitTransactionUpdate(updateTx)
		}
	}

	// Signal that the response can continue
	close(req.Done)
	// Don't remove from queue here - let handleInterceptedResponse handle cleanup
	// p.interceptQueue.Remove(id)
	return nil
}

// cleanupRequestMapping cleans up the request signature mapping
func (p *ProxyServer) cleanupRequestMapping(req *network.HTTPTransaction) {
	if req.InterceptedRequestID != "" {
		for sig, mappedID := range p.interceptedRequestMap {
			if mappedID == req.InterceptedRequestID {
				delete(p.interceptedRequestMap, sig)
				fmt.Printf("Debug: Cleaned up request signature mapping: %s\n", sig)
				break
			}
		}
	}
}

// ForwardInterceptedRequest forwards an intercepted request with optional response interception
func (p *ProxyServer) ForwardInterceptedRequest(id string, method, url string, headers map[string]string, rawRequest string, interceptResponse bool) error {
	p.interceptMutex.Lock()
	req, exists := p.interceptQueue.Get(id)
	if !exists {
		p.interceptMutex.Unlock()
		return fmt.Errorf("request not found")
	}

	// Update the request dump with the modified raw request
	req.Request.Dump = rawRequest

	// Set whether to intercept response or auto-forward
	req.InterceptResponse = interceptResponse
	req.WaitingForResponse = true
	req.InterceptedRequestID = id

	// If this is an auto-forward (not intercepting response), mark it as auto-forwarded
	// This will hide it from the UI but keep it in the queue for the proxy flow
	if !interceptResponse {
		req.AutoForwarded = true
		fmt.Printf("Debug: Marked request %s as auto-forwarded (hidden from UI)\n", id)
	}

	p.interceptMutex.Unlock()

	// Forward the request directly and get the response
	go p.executeForwardedRequest(req, method, url, rawRequest)

	return nil
}

// DropInterceptedRequest drops an intercepted request entirely without forwarding it
func (p *ProxyServer) DropInterceptedRequest(id string) error {
	p.interceptMutex.Lock()
	defer p.interceptMutex.Unlock()

	req, exists := p.interceptQueue.Get(id)
	if !exists {
		return fmt.Errorf("request not found")
	}

	// Mark the request as dropped
	req.WasDropped = true
	req.CameFrom = "intercept"

	// Update the corresponding history transaction to mark it as dropped
	if req.HistoryID != "" {
		// Create the update transaction with dropped flag
		updateTx := *req
		updateTx.ID = req.HistoryID // Use the original history ID
		updateTx.WasDropped = true
		updateTx.CameFrom = "intercept"

		// Update the transaction in the store
		err := p.transactionStore.Update(updateTx)
		if err != nil {
			fmt.Printf("Error updating dropped transaction in store: %v\n", err)
		} else {
			fmt.Printf("Debug: Marked history transaction %s as dropped\n", updateTx.ID)
		}

		// Emit event for updated transaction
		if p.eventEmitter != nil {
			p.eventEmitter.EmitTransactionUpdate(updateTx)
		}
	}

	// Remove from intercept queue and signal completion
	p.interceptQueue.Remove(id)
	close(req.Done)

	fmt.Printf("Debug: Dropped intercepted request %s\n", id)
	return nil
}

// executeForwardedRequest executes a forwarded intercepted request
func (p *ProxyServer) executeForwardedRequest(req *network.HTTPTransaction, method, url, rawRequest string) {
	// Parse the raw HTTP request to extract headers and body
	parsedHeaders, body, err := http_utils.ParseRawHTTPRequest(rawRequest)
	if err != nil {
		fmt.Printf("Error parsing raw HTTP request: %v\n", err)
		// Signal completion even on error
		close(req.Done)
		p.interceptQueue.Remove(req.ID)
		return
	}

	// Create HTTP client
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			DisableKeepAlives: true,
		},
		Timeout: 300 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects automatically
		},
	}

	// Create the request
	outReq, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		// Signal completion even on error
		close(req.Done)
		p.interceptQueue.Remove(req.ID)
		return
	}

	// Add parsed headers
	for key, value := range parsedHeaders {
		outReq.Header.Set(key, value)
	}

	// Send the request
	resp, err := client.Do(outReq)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		// Signal completion even on error
		close(req.Done)
		p.interceptQueue.Remove(req.ID)
		return
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)

	// Create response dump
	dumpResp := *resp
	dumpResp.Body = io.NopCloser(bytes.NewBuffer(respBody))
	responseDump, err := httputil.DumpResponse(&dumpResp, true)
	if err != nil {
		responseDump = []byte(fmt.Sprintf("Failed to dump response: %v", err))
	}

	// Print the raw intercepted response in hex format before any processing
	// fmt.Printf("=== RAW INTERCEPTED RESPONSE (HEX) ===\n%s\n=== END RAW RESPONSE ===\n", hex.EncodeToString(responseDump))

	// Update the intercepted request with the response
	p.interceptMutex.Lock()
	defer p.interceptMutex.Unlock()

	if interceptedReq, exists := p.interceptQueue.Get(req.ID); exists {
		if req.InterceptResponse {
			// User wants to intercept the response - update the intercept queue
			interceptedReq.Response = &network.HTTPResponse{
				Dump: p.processInterceptedResponseDump(responseDump),
			}
			interceptedReq.WaitingForResponse = false

			// Update the corresponding history transaction with the response
			if interceptedReq.HistoryID != "" {
				// Create the update transaction with proper response data
				updateTx := *interceptedReq
				updateTx.ID = interceptedReq.HistoryID // Use the original history ID

				// Ensure the response is properly set for history
				if updateTx.Response == nil {
					updateTx.Response = &network.HTTPResponse{}
				}
				updateTx.Response.Dump = p.processInterceptedResponseDump(responseDump)

				// Update the transaction in the store
				err := p.transactionStore.Update(updateTx)
				if err != nil {
					fmt.Printf("Error updating transaction in store: %v\n", err)
				} else {
					fmt.Printf("Debug: Updated history transaction %s with response %d\n", updateTx.ID, resp.StatusCode)
				}

				// Emit event for updated transaction
				if p.eventEmitter != nil {
					p.eventEmitter.EmitTransactionUpdate(updateTx)
				}
			}

			// Wait for user to modify response - Don't close Done yet
			// The Done channel will be closed when user forwards the response via ModifyInterceptedResponse
			fmt.Printf("Debug: Response received for intercepted request %s, waiting for user action\n", req.ID)
			// The browser connection is still active and waiting via the original proxy flow
		} else {
			// Auto-forward response transparently - set response data for proxy flow
			fmt.Printf("Debug: Auto-forwarding response transparently for request %s\n", req.ID)

			// Set the response data in the intercepted request so handleInterceptedResponse can use it
			interceptedReq.Response = &network.HTTPResponse{
				Dump: p.processInterceptedResponseDump(responseDump),
			}
			interceptedReq.WaitingForResponse = false

			// Update history if needed
			if interceptedReq.HistoryID != "" {
				// Create the update transaction with proper response data
				updateTx := *interceptedReq
				updateTx.ID = interceptedReq.HistoryID // Use the original history ID

				// Ensure the response is properly set for history
				if updateTx.Response == nil {
					updateTx.Response = &network.HTTPResponse{}
				}
				updateTx.Response.Dump = p.processInterceptedResponseDump(responseDump)

				// Update the transaction in the store
				err := p.transactionStore.Update(updateTx)
				if err != nil {
					fmt.Printf("Error updating transaction in store: %v\n", err)
				} else {
					fmt.Printf("Debug: Updated history transaction %s with response %d\n", updateTx.ID, resp.StatusCode)
				}

				// Emit event for updated transaction
				if p.eventEmitter != nil {
					p.eventEmitter.EmitTransactionUpdate(updateTx)
				}
			}

			// Signal completion for the original proxy flow
			close(interceptedReq.Done)
		}
	} else {
		// Request not found in queue - this shouldn't happen
		fmt.Printf("Error: Request %s not found in intercept queue\n", req.ID)
	}
}

// handleProxy handles incoming proxy requests
func (p *ProxyServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %s %s (Host: %s)\n", r.Method, r.URL, r.Host)

	// Check if the user is directly accessing the proxy server itself
	if http_utils.IsProxyHost(r.Host) {
		p.handleDirectProxyAccess(w, r)
		return
	}

	if r.Method == http.MethodConnect {
		p.handleConnectRequest(w, r)
	} else {
		p.handleHTTPRequest(w, r)
	}
}

// handleDirectProxyAccess handles requests directly to the proxy server
func (p *ProxyServer) handleDirectProxyAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "") {
		p.serveCertificatePage(w)
		return
	}

	if r.Method == http.MethodGet && r.URL.Path == "/download-ca" {
		p.serveCertificateDownload(w)
		return
	}
}

// serveCertificatePage serves the CA certificate installation page
func (p *ProxyServer) serveCertificatePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Gleip Certificate Authority</title>
    <style>
        :root {
            --primary: #4f46e5;
            --primary-dark: #4338ca;
            --gray-50: #f9fafb;
            --gray-100: #f3f4f6;
            --gray-700: #374151;
            --gray-800: #1f2937;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background-color: var(--gray-50);
            color: var(--gray-800);
            line-height: 1.5;
        }

        .container {
            text-align: center;
            padding: 2.5rem;
            background-color: white;
            border-radius: 12px;
            box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06);
            max-width: 1000px;
            width: 90%;
            margin: 2rem;
        }

        h1 {
            color: var(--gray-800);
            margin-bottom: 2rem;
            font-size: 2rem;
            font-weight: 600;
        }

        .download-btn {
            display: inline-flex;
            align-items: center;
            padding: 0.875rem 2rem;
            font-size: 1rem;
            font-weight: 500;
            color: white;
            background-color: var(--primary);
            border: none;
            border-radius: 6px;
            cursor: pointer;
            text-decoration: none;
            transition: all 0.2s ease;
            box-shadow: 0 1px 3px 0 rgba(0,0,0,0.1), 0 1px 2px 0 rgba(0,0,0,0.06);
        }

        .download-btn:hover {
            background-color: var(--primary-dark);
            transform: translateY(-1px);
            box-shadow: 0 4px 6px -1px rgba(0,0,0,0.1), 0 2px 4px -1px rgba(0,0,0,0.06);
        }

        .download-btn:active {
            transform: translateY(0);
        }

        .download-btn::before {
            content: "\2193";  /* Unicode down arrow */
            margin-right: 0.5rem;
            font-size: 1.1rem;
        }

        .arrow-right::after {
            content: "\2192";  /* Unicode right arrow */
        }

        .instructions {
            margin-top: 3rem;
            color: var(--gray-700);
            font-size: 0.95rem;
            max-width: 900px;
            text-align: left;
            margin-left: auto;
            margin-right: auto;
        }

        .instructions h3 {
            color: var(--gray-800);
            font-size: 1.25rem;
            margin-bottom: 1rem;
            font-weight: 600;
        }

        .instructions ol {
            padding-left: 1.5rem;
        }

        .instructions li {
            margin-bottom: 0.75rem;
            padding-left: 0.5rem;
        }

        .browser-specific {
            margin-top: 1.5rem;
            padding: 1rem;
            background-color: var(--gray-100);
            border-radius: 6px;
        }

        .browser-specific h4 {
            margin: 0 0 0.75rem 0;
            font-size: 1rem;
            font-weight: 600;
        }

        .browser-steps {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 1rem;
        }

        .browser-step {
            padding: 1rem;
            background-color: white;
            border-radius: 4px;
            box-shadow: 0 1px 2px 0 rgba(0,0,0,0.05);
        }

        .important-note {
            border-left: 4px solid var(--primary);
            padding: 0.5rem 1rem;
            margin: 1.5rem 0;
            background-color: rgba(79, 70, 229, 0.1);
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Gleip Certificate Authority</h1>
        <a href="/download-ca" class="download-btn">Download Certificate</a>
        
        <div class="instructions">
            <h3>Certificate Installation Instructions</h3>
            <p>To enable secure HTTPS inspection, you need to install and <strong>trust</strong> the Gleip Certificate Authority in your browser:</p>
            
            <div class="important-note">
                <p><strong>Important:</strong> You must install this certificate as a <strong>"trusted root certificate authority"</strong>, not just as a regular certificate. This will allow Gleip to generate trusted certificates for HTTPS websites.</p>
            </div>
            
            <ol>
                <li>Download the certificate using the button above</li>
                <li>Follow the browser-specific instructions below to install the certificate</li>
                <li>Make sure to select <strong>"Trust this certificate as a root authority"</strong> when prompted</li>
                <li>Restart your browser after installation</li>
            </ol>

            <div class="browser-specific">
                <h4>Browser-specific Installation Steps</h4>
                <div class="browser-steps">
                    <div class="browser-step">
                        <strong>Chrome / Edge (Windows)</strong>
                        <ol>
                            <li>Open Settings</li>
                            <li>Go to Privacy and Security</li>
                            <li>Click Security <span class="arrow-right"></span> Manage Certificates</li>
                            <li>Select <strong>Trusted Root Certification Authorities</strong> tab</li>
                            <li>Click Import and select the downloaded certificate</li>
                            <li>Follow the wizard and confirm all security warnings</li>
                        </ol>
                    </div>
                    <div class="browser-step">
                        <strong>Chrome / Edge (Mac)</strong>
                        <ol>
                            <li>Double-click the downloaded certificate</li>
                            <li>In Keychain Access, add to the System keychain</li>
                            <li>Find the certificate, double-click it</li>
                            <li>Expand "Trust" and set "When using this certificate" to <strong>"Always Trust"</strong></li>
                            <li>Close the window and enter your password to confirm</li>
                            <li>Restart your browser</li>
                        </ol>
                    </div>
                    <div class="browser-step">
                        <strong>Firefox</strong>
                        <ol>
                            <li>Open Settings / Preferences</li>
                            <li>Go to Privacy & Security</li>
                            <li>Scroll to Certificates</li>
                            <li>Click View Certificates <span class="arrow-right"></span> Authorities</li>
                            <li>Click Import and select the downloaded certificate</li>
                            <li>Check <strong>"Trust this CA to identify websites"</strong></li>
                            <li>Click OK and restart Firefox</li>
                        </ol>
                    </div>
                    <div class="browser-step">
                        <strong>Safari</strong>
                        <ol>
                            <li>Double-click the downloaded certificate</li>
                            <li>Select <strong>System</strong> in the Keychain dropdown</li>
                            <li>Click Add to import the certificate</li>
                            <li>Go to Keychain Access app</li>
                            <li>Find "Gleip Root Certificate Authority"</li>
                            <li>Double-click it, expand "Trust"</li>
                            <li>Set "When using this certificate" to <strong>"Always Trust"</strong></li>
                            <li>Restart Safari</li>
                        </ol>
                    </div>
                </div>
            </div>
            
            <div class="important-note" style="margin-top: 2rem;">
                <p><strong>Verification:</strong> After installation, when you visit an HTTPS website through Gleip, the certificate should show as "Verified by: Gleip" in the browser's certificate viewer.</p>
            </div>
        </div>
    </div>
</body>
</html>`
	w.Write([]byte(html))
}

// serveCertificateDownload serves the CA certificate for download
func (p *ProxyServer) serveCertificateDownload(w http.ResponseWriter) {
	certPath := filepath.Join(p.certManager.GetCAPath(), "gleip.cer")
	certData, err := os.ReadFile(certPath)
	if err != nil {
		http.Error(w, "Certificate not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", `attachment; filename="gleip.cer"`)
	w.Write(certData)
}

// handleConnectRequest handles CONNECT requests for HTTPS connections
func (p *ProxyServer) handleConnectRequest(w http.ResponseWriter, r *http.Request) {
	// Extract host from CONNECT request
	host := r.Host
	if !strings.Contains(host, ":") {
		host = host + ":443"
	}

	// Extract hostname (without port) for certificate
	hostname := strings.Split(host, ":")[0]

	fmt.Printf("[HTTPS] Processing CONNECT request for: %s\n", hostname)

	// Pre-generate certificate to ensure it's in the cache when needed
	_, err := p.certManager.GenerateCertificate(hostname)
	if err != nil {
		fmt.Printf("[ERROR] Failed to generate certificate for %s: %v\n", hostname, err)
		http.Error(w, fmt.Sprintf("Failed to generate certificate: %v", err), http.StatusInternalServerError)
		return
	}

	// Create TLS config for the client connection with SNI support
	tlsConfig := &tls.Config{
		GetCertificate: p.certManager.GetCertificateForConn,
		MinVersion:     tls.VersionTLS12,
		NextProtos:     []string{"h2", "http/1.1"},
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
	}

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		fmt.Printf("[ERROR] Hijacking not supported for %s\n", hostname)
		http.Error(w, "Hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		fmt.Printf("[ERROR] Failed to hijack connection for %s: %v\n", hostname, err)
		http.Error(w, fmt.Sprintf("Failed to hijack connection: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	// Send 200 OK to establish the tunnel
	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		fmt.Printf("[ERROR] Failed to send connection established for %s: %v\n", hostname, err)
		return
	}

	fmt.Printf("[HTTPS] Tunnel established for %s, initiating TLS handshake\n", hostname)

	// Wrap client connection with TLS
	tlsClientConn := tls.Server(clientConn, tlsConfig)
	if err := tlsClientConn.Handshake(); err != nil {
		fmt.Printf("[ERROR] TLS handshake failed for %s: %v\n", hostname, err)
		tlsClientConn.Close()
		return
	}
	defer tlsClientConn.Close()

	fmt.Printf("[HTTPS] TLS handshake successful for %s\n", hostname)

	// Handle HTTPS requests
	p.handleHTTPSConnection(tlsClientConn, r)
}

// handleHTTPSConnection handles the HTTPS connection after TLS handshake
func (p *ProxyServer) handleHTTPSConnection(tlsConn *tls.Conn, originalRequest *http.Request) {
	// Create a context with timeout for the HTTPS server
	serverCtx, serverCancel := context.WithCancel(originalRequest.Context())
	defer serverCancel()

	// Create the HTTP transport once for reuse
	transport := p.createHTTPTransport()
	client := p.createHTTPClient(transport)

	// Create a new HTTP server to handle HTTPS requests with timeouts
	httpsServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set the full URL for the request
			r.URL.Scheme = "https"
			r.URL.Host = r.Host

			// Handle the request using the common handler
			p.handleHTTPRequestWithClient(w, r, client, true)
		}),
		ErrorLog: nil,
		// Add timeouts
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Serve HTTPS requests and ensure cleanup
	go func() {
		httpsServer.Serve(newOneConnListener(tlsConn))
		serverCancel() // Cancel context when done
		tlsConn.Close()
	}()

	// Wait for context cancellation
	<-serverCtx.Done()
}

// handleHTTPRequest handles regular HTTP requests
func (p *ProxyServer) handleHTTPRequest(w http.ResponseWriter, r *http.Request) {
	// Ensure the URL is absolute
	if !r.URL.IsAbs() {
		// For proxy requests, the RequestURI contains the full URL
		if r.RequestURI != "" {
			parsedURL, err := url.Parse(r.RequestURI)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to parse URL: %v", err), http.StatusBadRequest)
				return
			}
			r.URL = parsedURL
		} else if r.Host != "" {
			r.URL.Scheme = "http"
			r.URL.Host = r.Host
		}
	}

	fmt.Printf("Forwarding to: %s\n", r.URL.String())

	// Create the HTTP transport and client
	transport := p.createHTTPTransport()
	client := p.createHTTPClient(transport)

	// Handle the request using the common handler
	p.handleHTTPRequestWithClient(w, r, client, false)
}

// handleHTTPRequestWithClient handles both HTTP and HTTPS requests with a given client
func (p *ProxyServer) handleHTTPRequestWithClient(w http.ResponseWriter, r *http.Request, client *http.Client, isHTTPS bool) {
	// Read request body
	reqBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

	// Get raw request dump
	requestDump, err := httputil.DumpRequest(r, true)
	if err != nil {
		requestDump = []byte(fmt.Sprintf("Failed to dump request: %v", err))
	}

	var intercepted *network.HTTPTransaction
	var wasIntercepted bool

	// Check if interception is enabled for requests
	if p.interceptEnabled {
		intercepted = p.interceptRequest(r, requestDump, isHTTPS)
		if intercepted == nil {
			return // Request was handled by interception
		}
		wasIntercepted = true

		// Check if the request was dropped by trying to get it from the queue
		// DropInterceptedRequest removes the request from the queue, so if it's not there, it was dropped
		if updatedIntercepted, exists := p.interceptQueue.Get(intercepted.ID); !exists {
			// Request was dropped - send error response to browser
			w.WriteHeader(444) // 444 is "No Response" - nginx extension, but widely understood
			w.Write([]byte("Request was dropped by proxy"))
			fmt.Printf("Debug: Sent 444 No Response for dropped request %s\n", intercepted.ID)
			return
		} else {
			// Request is still in queue, use the updated version
			intercepted = updatedIntercepted

			// Double-check the WasDropped flag (additional safety check)
			if intercepted.WasDropped {
				w.WriteHeader(444)
				w.Write([]byte("Request was dropped by proxy"))
				fmt.Printf("Debug: Sent 444 No Response for request marked as dropped %s\n", intercepted.ID)
				return
			}
		}

		// Check if this request has actual response data (from ForwardRequestAndWaitForResponse)
		if p.handleInterceptedResponse(w, intercepted) {
			return
		}

		// Update request with modified values for normal flow
		r.Method = intercepted.Request.Method()
		r.Host = intercepted.Request.Host
		r.URL, _ = url.Parse(intercepted.Request.URL())
		r.Header = make(http.Header)
		r.Body = io.NopCloser(bytes.NewBuffer(intercepted.Request.Body()))
	}

	// Forward request to destination
	resp, respBody, err := p.forwardRequest(r, client)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Get raw response dump
	responseDump := p.dumpResponse(resp, respBody)

	// Only intercept responses if this was specifically requested (not for all requests when interception is enabled)
	// This will be handled by executeForwardedRequest for forwarded requests that need response interception

	// Create or update transaction
	if wasIntercepted && intercepted.HistoryID != "" {
		// Update the existing intercepted transaction with the response
		updateTx := *intercepted
		updateTx.ID = intercepted.HistoryID // Use the original history ID

		// Set the response data
		updateTx.Response = &network.HTTPResponse{
			Dump: string(responseDump),
		}

		// Update the transaction in the store
		err := p.transactionStore.Update(updateTx)
		if err != nil {
			fmt.Printf("Error updating intercepted transaction in store: %v\n", err)
		} else {
			fmt.Printf("Debug: Updated intercepted transaction %s with response %d\n", updateTx.ID, resp.StatusCode)
		}

		// Emit event for updated transaction
		if p.eventEmitter != nil {
			p.eventEmitter.EmitTransactionUpdate(updateTx)
		}
	} else {
		// Create and store new transaction (for non-intercepted requests)
		p.storeTransaction(r, requestDump, resp, responseDump, isHTTPS)
	}

	// Forward response to client
	p.forwardResponse(w, resp, respBody)
}

// interceptRequest handles request interception
func (p *ProxyServer) interceptRequest(r *http.Request, requestDump []byte, isHTTPS bool) *network.HTTPTransaction {
	// Create headers map
	headers := make(map[string]string)
	for key, values := range r.Header {
		headers[key] = values[0]
	}

	// Create interception request
	intercepted := &network.HTTPTransaction{
		ID: uuid.New().String(),
		Request: network.HTTPRequest{
			Host: r.Host,
			Dump: string(requestDump),
			TLS:  isHTTPS,
		},
		Timestamp: time.Now().Format(time.RFC3339),
		Done:      make(chan struct{}),
		CameFrom:  "browser",
	}

	// Add intercepted request to history immediately (without response)
	p.transactionStore.Add(*intercepted)

	// Store the history transaction ID in the intercepted request for later updates
	intercepted.HistoryID = intercepted.ID

	if p.eventEmitter != nil {
		p.eventEmitter.EmitNewTransaction(*intercepted)
	}

	// Add to intercept queue
	p.interceptQueue.Add(intercepted)

	// Wait for modification/approval
	<-intercepted.Done

	return intercepted
}

// handleInterceptedResponse handles intercepted response data
func (p *ProxyServer) handleInterceptedResponse(w http.ResponseWriter, intercepted *network.HTTPTransaction) bool {
	hasResponse := intercepted.Response != nil && intercepted.Response.Dump != ""

	if hasResponse {
		// Use the response dump as the single source of truth
		fmt.Printf("Debug: Using response dump for intercepted request %s\n", intercepted.ID)

		// Parse headers from the response dump
		headers := intercepted.Response.Headers()
		for key, value := range headers {
			w.Header().Set(key, value)
		}

		// Set status code from the response dump
		statusCode := intercepted.Response.StatusCode()
		w.WriteHeader(statusCode)

		// Write body from the response dump
		body := intercepted.Response.Body()
		w.Write(body)

		// Update the existing transaction in history with the response data
		if intercepted.HistoryID != "" {
			// Create the update transaction with proper response data
			updateTx := *intercepted
			updateTx.ID = intercepted.HistoryID // Use the original history ID

			// Ensure the response is properly set for history
			if updateTx.Response == nil {
				updateTx.Response = &network.HTTPResponse{}
			}
			updateTx.Response.Dump = intercepted.Response.Dump

			// Update the transaction in the store
			err := p.transactionStore.Update(updateTx)
			if err != nil {
				fmt.Printf("Error updating transaction in store: %v\n", err)
			} else {
				fmt.Printf("Debug: Updated history transaction %s with response %d\n", updateTx.ID, intercepted.Response.StatusCode())
			}

			// Emit event for updated transaction
			if p.eventEmitter != nil {
				p.eventEmitter.EmitTransactionUpdate(updateTx)
			}
		}

		// Clean up the intercepted request from the queue
		p.interceptQueue.Remove(intercepted.ID)

		return true
	}

	return false
}

// forwardRequest forwards the request to the destination server
func (p *ProxyServer) forwardRequest(r *http.Request, client *http.Client) (*http.Response, []byte, error) {
	reqBody, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewBuffer(reqBody))

	outReq, err := http.NewRequest(r.Method, r.URL.String(), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, err
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			outReq.Header.Add(key, value)
		}
	}

	resp, err := client.Do(outReq)
	if err != nil {
		return nil, nil, err
	}

	// Read response body
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Create a new body reader for further processing
	resp.Body = io.NopCloser(bytes.NewBuffer(respBody))

	return resp, respBody, nil
}

// dumpResponse creates a response dump
func (p *ProxyServer) dumpResponse(resp *http.Response, respBody []byte) []byte {
	// Create a new response for dumping (with the body)
	dumpResp := *resp
	dumpResp.Body = io.NopCloser(bytes.NewBuffer(respBody))

	// Get raw response dump
	responseDump, err := httputil.DumpResponse(&dumpResp, true)
	if err != nil {
		responseDump = []byte(fmt.Sprintf("Failed to dump response: %v", err))
	}

	return responseDump
}

// processInterceptedResponseDump processes a response dump based on the decompression setting
func (p *ProxyServer) processInterceptedResponseDump(responseDump []byte) string {
	// Determine if decompression should be applied
	shouldDecompress := true // Always decompress for intercepted responses

	// Always decode chunked and correctly recalculate Content-Length
	return http_utils.GetPrintableResponseWithDecompression(responseDump, shouldDecompress)
}

// storeTransaction creates and stores a transaction
func (p *ProxyServer) storeTransaction(r *http.Request, requestDump []byte, resp *http.Response, responseDump []byte, isHTTPS bool) {
	// Create proxy request record
	proxyReq := network.HTTPTransaction{
		ID: uuid.New().String(),
		Request: network.HTTPRequest{
			Host: r.Host,
			Dump: string(requestDump),
			TLS:  isHTTPS,
		},
		Response: &network.HTTPResponse{
			Dump: string(responseDump),
		},
		Timestamp: time.Now().Format(time.RFC3339),
		SeqNumber: p.transactionStore.GetNextSequenceNumber(),
		CameFrom:  "browser",
	}

	p.transactionStore.Add(proxyReq)

	// Call the transaction callback if set
	if p.eventEmitter != nil {
		p.eventEmitter.EmitNewTransaction(proxyReq)
	}
}

// forwardResponse forwards the response to the client
func (p *ProxyServer) forwardResponse(w http.ResponseWriter, resp *http.Response, respBody []byte) {
	// Forward response headers to client
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}

// createHTTPTransport creates a standardized HTTP transport
func (p *ProxyServer) createHTTPTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		// Close connections after use
		DisableKeepAlives: true,
		// Set timeouts
		DialContext: (&net.Dialer{
			Timeout:   300 * time.Second,
			KeepAlive: 300 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       300 * time.Second,
		MaxIdleConns:          300,
		MaxIdleConnsPerHost:   100,
	}
}

// createHTTPClient creates a standardized HTTP client
func (p *ProxyServer) createHTTPClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport: transport,
		Timeout:   300 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// AddTransactionToHistory adds a transaction that originated from outside the proxy
func (p *ProxyServer) AddTransactionToHistory(transaction network.HTTPTransaction) {
	transaction.SeqNumber = p.transactionStore.GetNextSequenceNumber()
	// Assign a new ID to ensure uniqueness within the proxy history
	transaction.ID = uuid.New().String()
	// Set the cameFrom attribute to indicate this came from a gleipflow
	transaction.CameFrom = "gleipflow"

	p.transactionStore.Add(transaction)

	// Emit event so UI can update
	if p.eventEmitter != nil {
		p.eventEmitter.EmitNewTransaction(transaction)
	}

	fmt.Printf("Added transaction from gleip to history: ID %s, Seq %d, URL %s\n", transaction.ID, transaction.SeqNumber, transaction.Request.URL())
}
