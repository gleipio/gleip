package network

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/andybalholm/brotli"
)

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
	for _, line := range headerLines {
		if bytes.HasPrefix(line, []byte("Transfer-Encoding:")) {
			transferEncoding = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("Transfer-Encoding:"))))
		}
		if bytes.HasPrefix(line, []byte("Content-Encoding:")) {
			contentEncoding = string(bytes.TrimSpace(bytes.TrimPrefix(line, []byte("Content-Encoding:"))))
		}
	}

	// First handle chunked encoding if present
	if strings.EqualFold(transferEncoding, "chunked") {
		decodedBody, err := decodeChunkedBody(bufio.NewReader(bytes.NewReader(body)))
		if err != nil {
			return fmt.Sprintf("Failed to decode chunked response: %v", err)
		}
		body = decodedBody
	}

	// Then handle compression based on Content-Encoding
	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return fmt.Sprintf("Failed to decompress gzip response: %v", err)
		}
		defer reader.Close()

		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Sprintf("Failed to read decompressed gzip response: %v", err)
		}
		body = decompressed

	case "br", "brotli":
		reader := brotli.NewReader(bytes.NewReader(body))
		decompressed, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Sprintf("Failed to decompress brotli response: %v", err)
		}
		body = decompressed

	default:
		// Check for gzip magic bytes as fallback
		if isGzipped(body) {
			reader, err := gzip.NewReader(bytes.NewReader(body))
			if err != nil {
				return fmt.Sprintf("Failed to decompress response: %v", err)
			}
			defer reader.Close()

			decompressed, err := io.ReadAll(reader)
			if err != nil {
				return fmt.Sprintf("Failed to read decompressed response: %v", err)
			}
			body = decompressed
		}
	}

	// Return the headers and processed body
	return string(headers) + "\r\n\r\n" + string(body)
}

// isGzipped checks if data is gzipped by looking at magic bytes
func isGzipped(data []byte) bool {
	return len(data) > 2 && data[0] == 0x1f && data[1] == 0x8b
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
