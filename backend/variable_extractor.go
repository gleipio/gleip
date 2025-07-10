package backend

import (
	"Gleip/backend/network"
	"fmt"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

// DefaultVariableExtractor implements the VariableExtractor interface
type DefaultVariableExtractor struct{}

// NewVariableExtractor creates a new variable extractor
func NewVariableExtractor() VariableExtractor {
	return &DefaultVariableExtractor{}
}

// Extract extracts a variable from a response based on the extraction configuration
func (e *DefaultVariableExtractor) Extract(extract VariableExtract, transaction *network.HTTPTransaction) (string, error) {
	if transaction.Response == nil {
		return "", fmt.Errorf("no response available")
	}

	switch extract.Source {
	case "status":
		return e.extractFromStatus(transaction)
	case "header":
		return e.extractFromHeader(extract.Selector, transaction)
	case "cookie":
		return e.extractFromCookie(extract.Selector, transaction)
	case "body-json":
		return e.extractFromBodyJSON(extract.Selector, transaction)
	case "body-regex":
		return e.extractFromBodyRegex(extract.Selector, transaction)
	default:
		return "", fmt.Errorf("unknown source: %s", extract.Source)
	}
}

func (e *DefaultVariableExtractor) extractFromStatus(transaction *network.HTTPTransaction) (string, error) {
	return fmt.Sprintf("%d", transaction.Response.StatusCode()), nil
}

func (e *DefaultVariableExtractor) extractFromHeader(headerName string, transaction *network.HTTPTransaction) (string, error) {
	if transaction.Response.Dump == "" {
		return "", fmt.Errorf("response headers not available")
	}

	// Parse headers from the dump
	headerLines := strings.Split(transaction.Response.Dump, "\r\n")
	for _, line := range headerLines {
		if line == "" || !strings.Contains(line, ":") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		currentHeaderName := strings.TrimSpace(parts[0])
		headerValue := strings.TrimSpace(parts[1])

		if strings.EqualFold(currentHeaderName, headerName) {
			return headerValue, nil
		}
	}

	return "", fmt.Errorf("header not found: %s", headerName)
}

func (e *DefaultVariableExtractor) extractFromCookie(cookieName string, transaction *network.HTTPTransaction) (string, error) {
	if transaction.Response.Dump == "" {
		return "", fmt.Errorf("response headers not available")
	}

	// Parse headers from the dump to find Set-Cookie headers
	headerLines := strings.Split(transaction.Response.Dump, "\r\n")
	for _, line := range headerLines {
		if line == "" || !strings.HasPrefix(strings.ToLower(line), "set-cookie:") {
			continue
		}

		// Extract the cookie part
		cookiePart := strings.TrimSpace(strings.TrimPrefix(line, "Set-Cookie:"))
		cookiePart = strings.TrimSpace(strings.TrimPrefix(cookiePart, "set-cookie:")) // Case insensitive

		// Parse the cookie string
		cookiePrefix := cookieName + "="

		if strings.HasPrefix(cookiePart, cookiePrefix) {
			// Extract the value until ; or end of string
			valueEndPos := strings.Index(cookiePart[len(cookiePrefix):], ";")
			if valueEndPos == -1 {
				// No semicolon, take the whole value
				return cookiePart[len(cookiePrefix):], nil
			} else {
				// Return value until semicolon
				return cookiePart[len(cookiePrefix) : len(cookiePrefix)+valueEndPos], nil
			}
		} else if strings.Contains(cookiePart, "; "+cookiePrefix) {
			// The cookie might be in the middle of the string
			startPos := strings.Index(cookiePart, "; "+cookiePrefix) + len("; "+cookiePrefix)
			// Extract the value until ; or end of string
			valueEndPos := strings.Index(cookiePart[startPos:], ";")
			if valueEndPos == -1 {
				// No semicolon, take the whole value
				return cookiePart[startPos:], nil
			} else {
				// Return value until semicolon
				return cookiePart[startPos : startPos+valueEndPos], nil
			}
		}
	}

	return "", fmt.Errorf("cookie not found: %s", cookieName)
}

func (e *DefaultVariableExtractor) extractFromBodyJSON(jsonPath string, transaction *network.HTTPTransaction) (string, error) {
	if transaction.Response.Printable() == "" {
		return "", fmt.Errorf("response body not available")
	}

	// Use gjson to extract the value
	result := gjson.Get(transaction.Response.Printable(), jsonPath)
	if !result.Exists() {
		return "", fmt.Errorf("JSON path not found: %s", jsonPath)
	}

	return result.String(), nil
}

func (e *DefaultVariableExtractor) extractFromBodyRegex(pattern string, transaction *network.HTTPTransaction) (string, error) {
	if transaction.Response.Printable() == "" {
		return "", fmt.Errorf("response body not available")
	}

	// Compile and execute the regex
	reg, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("invalid regex: %v", err)
	}

	matches := reg.FindStringSubmatch(transaction.Response.Printable())
	if len(matches) == 0 {
		return "", fmt.Errorf("regex pattern not found: %s", pattern)
	}

	// If the regex has capture groups, return the first group
	if len(matches) > 1 {
		return matches[1], nil
	}

	// Otherwise return the entire match
	return matches[0], nil
}
