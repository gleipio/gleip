package network

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
)

// DefaultResponseDecompressor implements the ResponseDecompressor interface
type DefaultResponseDecompressor struct{}

// NewResponseDecompressor creates a new response decompressor
func NewResponseDecompressor() ResponseDecompressor {
	return &DefaultResponseDecompressor{}
}

// Decompress decompresses a compressed body based on content encoding
func (d *DefaultResponseDecompressor) Decompress(body []byte, contentEncoding string) ([]byte, error) {
	// Handle based on content encoding header first
	switch strings.ToLower(contentEncoding) {
	case "gzip":
		return d.decompressGzip(body)
	case "br", "brotli":
		return d.decompressBrotli(body)
	default:
		// Fallback to magic bytes detection for gzip
		if d.isGzip(body) {
			return d.decompressGzip(body)
		}
		return nil, fmt.Errorf("unsupported compression format or not compressed")
	}
}

// decompressGzip decompresses gzip-compressed data
func (d *DefaultResponseDecompressor) decompressGzip(compressedBody []byte) ([]byte, error) {
	// Check for gzip magic bytes
	if !d.isGzip(compressedBody) {
		return nil, fmt.Errorf("not a valid gzip format (missing magic bytes)")
	}

	// Create a gzip reader
	gzReader, err := gzip.NewReader(bytes.NewReader(compressedBody))
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	// Read the decompressed content
	result, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// decompressBrotli decompresses brotli-compressed data
func (d *DefaultResponseDecompressor) decompressBrotli(compressedBody []byte) ([]byte, error) {
	// Create a brotli reader
	brReader := brotli.NewReader(bytes.NewReader(compressedBody))

	// Read the decompressed content
	result, err := io.ReadAll(brReader)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// isGzip checks if data is gzipped by looking at magic bytes
func (d *DefaultResponseDecompressor) isGzip(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}
