package http_utils

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

// HTTPClient interface for HTTP operations (Dependency Inversion)
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// isProxyHost checks if the host is the proxy itself
func IsProxyHost(host string) bool {
	// Common patterns for direct access to the proxy
	return host == "127.0.0.1:9090" ||
		host == "localhost:9090" ||
		host == "127.0.0.1" ||
		host == "localhost"
}

// parseRawHTTPRequest parses a raw HTTP request string and extracts headers and body
func ParseRawHTTPRequest(rawRequest string) (map[string]string, string, error) {
	lines := strings.Split(rawRequest, "\n")
	if len(lines) == 0 {
		return nil, "", fmt.Errorf("empty request")
	}

	headers := make(map[string]string)
	var bodyLines []string
	inBody := false

	for i, line := range lines {
		line = strings.TrimRight(line, "\r") // Remove carriage return

		if i == 0 {
			// Skip the request line (GET /path HTTP/1.1)
			continue
		}

		if !inBody {
			if line == "" {
				// Empty line indicates start of body
				inBody = true
				continue
			}

			// Parse header line
			if colonIndex := strings.Index(line, ":"); colonIndex > 0 {
				key := strings.TrimSpace(line[:colonIndex])
				value := strings.TrimSpace(line[colonIndex+1:])
				headers[key] = value
			}
		} else {
			// Collect body lines
			bodyLines = append(bodyLines, line)
		}
	}

	body := strings.Join(bodyLines, "\n")
	return headers, body, nil
}

// GetPrintableResponse processes a response dump to make it human-readable
func GetPrintableResponse(responseDump []byte) string {
	return GetPrintableResponseWithDecompression(responseDump, true)
}

// GetPrintableResponseWithDecompression processes a response dump with optional decompression
func GetPrintableResponseWithDecompression(responseDump []byte, shouldDecompress bool) string {
	// Split the response into headers and body
	parts := bytes.SplitN(responseDump, []byte("\r\n\r\n"), 2)
	if len(parts) != 2 {
		return string(responseDump) // Return as-is if we can't split
	}

	headers := parts[0]
	body := parts[1]

	// Parse headers manually
	headerLines := bytes.Split(headers, []byte("\r\n"))
	var transferEncoding, contentEncoding string
	var modifiedHeaders [][]byte

	for _, line := range headerLines {
		if bytes.HasPrefix(line, []byte("Transfer-Encoding:")) {
			transferEncoding = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("Transfer-Encoding:"))))
			// Skip adding Transfer-Encoding header to modified headers if it's chunked
			if !strings.EqualFold(transferEncoding, "chunked") {
				modifiedHeaders = append(modifiedHeaders, line)
			}
		} else if bytes.HasPrefix(line, []byte("Content-Encoding:")) {
			contentEncoding = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("Content-Encoding:"))))
			// Skip adding Content-Encoding header if we're going to decompress
			if !shouldDecompress {
				modifiedHeaders = append(modifiedHeaders, line)
			}
		} else if bytes.HasPrefix(line, []byte("Content-Length:")) {
			// Skip original Content-Length if we're decoding chunked, we'll add a new one
			// For compression, only skip if we're actually decompressing
			skipContentLength := strings.EqualFold(transferEncoding, "chunked") ||
				(shouldDecompress && contentEncoding != "")
			if !skipContentLength {
				modifiedHeaders = append(modifiedHeaders, line)
			}
		} else {
			modifiedHeaders = append(modifiedHeaders, line)
		}
	}

	// First handle chunked encoding if present AND if the body is actually in chunked format
	wasChunked := strings.EqualFold(transferEncoding, "chunked")
	if wasChunked && isActuallyChunked(body) {
		decodedBody, err := decodeChunkedBody(bufio.NewReader(bytes.NewReader(body)))
		if err != nil {
			return fmt.Sprintf("Failed to decode chunked response: %v", err)
		}
		body = decodedBody
	} else if wasChunked && !isActuallyChunked(body) {
		// Header says chunked but body is already decoded (e.g., by httputil.DumpResponse)
		// Just remove the Transfer-Encoding header and add Content-Length
		wasChunked = true // Keep this true so we add Content-Length header
	}

	// Then handle compression based on Content-Encoding (only if decompression is enabled)
	var decompressionFailed = false
	if shouldDecompress {
		switch strings.ToLower(contentEncoding) {
		case "gzip":
			reader, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				fmt.Printf("Failed to decompress gzip response: %v...\n", hex.EncodeToString(body)[:20])
				fmt.Printf("Falling back to showing raw response without decompression\n")
				decompressionFailed = true
			} else {
				defer reader.Close()
				decompressed, err := io.ReadAll(reader)
				if err != nil {
					fmt.Printf("Failed to read decompressed gzip response: %v\n", err)
					fmt.Printf("Falling back to showing raw response without decompression\n")
					decompressionFailed = true
				} else {
					body = decompressed
				}
			}

		case "br", "brotli":
			reader := brotli.NewReader(bytes.NewReader(body))
			decompressed, err := io.ReadAll(reader)
			if err != nil {
				fmt.Printf("Failed to decompress brotli response: %v\n", err)
				fmt.Printf("Falling back to showing raw response without decompression\n")
				decompressionFailed = true
			} else {
				body = decompressed
			}

		default:
			// Check for gzip magic bytes as fallback
			if isGzipped(body) {
				reader, err := gzip.NewReader(bytes.NewReader(body))
				if err != nil {
					fmt.Printf("Failed to decompress response: %v\n", err)
					fmt.Printf("Falling back to showing raw response without decompression\n")
					decompressionFailed = true
				} else {
					defer reader.Close()
					decompressed, err := io.ReadAll(reader)
					if err != nil {
						fmt.Printf("Failed to read decompressed response: %v\n", err)
						fmt.Printf("Falling back to showing raw response without decompression\n")
						decompressionFailed = true
					} else {
						body = decompressed
					}
				}
			}
		}
	}

	// If decompression failed, add back the Content-Encoding header since we're showing the raw compressed content
	if decompressionFailed && contentEncoding != "" {
		contentEncodingHeader := fmt.Sprintf("Content-Encoding: %s", contentEncoding)
		modifiedHeaders = append(modifiedHeaders, []byte(contentEncodingHeader))
	}

	// Add Content-Length header only when we've modified the body
	// - Always add when we decoded chunked (body changed from chunks to raw data)
	// - Add when we decompressed successfully (body changed from compressed to uncompressed)
	// - Don't add if decompression failed (keep original Content-Length for compressed data)
	needsContentLength := wasChunked || (shouldDecompress && contentEncoding != "" && !decompressionFailed)
	if needsContentLength {
		actualBodyLength := len(body)
		contentLengthHeader := fmt.Sprintf("Content-Length: %d", actualBodyLength)
		modifiedHeaders = append(modifiedHeaders, []byte(contentLengthHeader))
	}

	// Return the modified headers and processed body
	finalHeaders := headers
	if len(modifiedHeaders) > 0 {
		finalHeaders = bytes.Join(modifiedHeaders, []byte("\r\n"))
	}

	// Build the complete response as bytes to preserve binary data, then convert to string
	result := make([]byte, 0, len(finalHeaders)+4+len(body))
	result = append(result, finalHeaders...)
	result = append(result, []byte("\r\n\r\n")...)
	result = append(result, body...)
	return string(result)
}

// isGzipped checks if data is gzipped by looking at magic bytes
func isGzipped(data []byte) bool {
	return len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b
}

// isActuallyChunked checks if the body data is actually in chunked encoding format
func isActuallyChunked(body []byte) bool {
	if len(body) == 0 {
		return false
	}

	// Chunked encoding starts with a hex number followed by CRLF
	// Look for the first line which should be a hex chunk size
	reader := bufio.NewReader(bytes.NewReader(body))
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	line = strings.TrimSpace(line)

	// Try to parse as hex - if it fails, it's not chunked format
	_, err = strconv.ParseInt(line, 16, 64)
	return err == nil
}

// decodeChunkedBody decodes chunked transfer encoding
func decodeChunkedBody(reader *bufio.Reader) ([]byte, error) {
	var result bytes.Buffer
	var err error

	for {
		// Read chunk size line
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk size: %v", err)
		}
		line = strings.TrimSpace(line)

		// Convert hex string to int
		chunkSize, err := strconv.ParseInt(line, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid chunk size: %s", line)
		}

		// Check for last chunk
		if chunkSize == 0 {
			break
		}

		// Read chunk data
		chunk := make([]byte, chunkSize)
		_, err = io.ReadFull(reader, chunk)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk data: %v", err)
		}
		result.Write(chunk)

		// Read and discard CRLF
		_, err = reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk CRLF: %v", err)
		}
	}

	// Read final CRLF
	_, err = reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("failed to read final CRLF: %v", err)
	}

	return result.Bytes(), nil
}

// splitRawRequest splits a raw HTTP request into headers and body
func SplitRawRequest(rawRequest string) (string, string) {
	normalizedRequest := strings.ReplaceAll(rawRequest, "\r\n", "\n")
	normalizedRequest = strings.ReplaceAll(normalizedRequest, "\n", "\r\n")

	parts := strings.SplitN(normalizedRequest, "\r\n\r\n", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// updateContentLengthInRawRequest updates the Content-Length header in a raw request
func UpdateContentLengthInRawRequest(rawRequest string, newBodyLength int) string {
	headersPart, bodyPart := SplitRawRequest(rawRequest)

	headerLines := strings.Split(headersPart, "\r\n")
	var newHeaderLines []string
	contentLengthFound := false

	if len(headerLines) > 0 {
		newHeaderLines = append(newHeaderLines, headerLines[0])
		for i := 1; i < len(headerLines); i++ {
			if strings.HasPrefix(strings.ToLower(headerLines[i]), "content-length:") {
				contentLengthFound = true
				if newBodyLength > 0 {
					newHeaderLines = append(newHeaderLines, fmt.Sprintf("Content-Length: %d", newBodyLength))
				}
			} else {
				newHeaderLines = append(newHeaderLines, headerLines[i])
			}
		}
	}

	if !contentLengthFound && newBodyLength > 0 {
		newHeaderLines = append(newHeaderLines, fmt.Sprintf("Content-Length: %d", newBodyLength))
	}

	updatedHeaders := strings.Join(newHeaderLines, "\r\n")
	return updatedHeaders + "\r\n\r\n" + bodyPart
}
