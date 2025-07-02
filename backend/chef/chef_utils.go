package chef

import (
	"Gleip/backend/gleipflow"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
)

// ChefAction represents a single transformation action in a chef step
type ChefAction struct {
	ID         string                 `json:"id"`
	ActionType string                 `json:"actionType"` // "base64_encode", "base64_decode", etc.
	Options    map[string]interface{} `json:"options"`    // Action-specific options
	Preview    string                 `json:"preview"`    // Preview of the transformation result
}

// ChefStep represents a complete chef step with input, actions, and output
type ChefStep struct {
	StepAttributes gleipflow.StepAttributes `json:"stepAttributes"`
	InputVariable  string                   `json:"inputVariable"`  // Name of the input variable
	Actions        []ChefAction             `json:"actions"`        // List of transformation actions
	OutputVariable string                   `json:"outputVariable"` // Name of the output variable
}

// ExecuteChefStep executes all actions in sequence and returns the final result
func ExecuteChefStep(step *ChefStep, inputValue string) (string, error) {
	if step == nil {
		return "", fmt.Errorf("chef step is nil")
	}

	result := inputValue

	// Execute each action in sequence
	for i, action := range step.Actions {
		var err error
		result, err = executeAction(action, result)
		if err != nil {
			return "", fmt.Errorf("error executing action %d (%s): %v", i+1, action.ActionType, err)
		}
	}

	return result, nil
}

// executeAction executes a single chef action
func executeAction(action ChefAction, input string) (string, error) {
	// Handle empty action type
	if action.ActionType == "" {
		return input, nil
	}

	switch action.ActionType {
	case "base64_encode":
		return executeBase64Encode(input)
	case "base64_decode":
		return executeBase64Decode(input)
	case "url_encode":
		return executeURLEncode(input)
	case "url_decode":
		return executeURLDecode(input)
	case "html_encode":
		return executeHTMLEncode(input)
	case "html_decode":
		return executeHTMLDecode(input)
	case "hex_encode":
		return executeHexEncode(input)
	case "hex_decode":
		return executeHexDecode(input)
	case "md5_hash":
		return executeMD5Hash(input)
	case "sha256_hash":
		return executeSHA256Hash(input)
	case "to_uppercase":
		return executeToUppercase(input)
	case "to_lowercase":
		return executeToLowercase(input)
	case "reverse_string":
		return executeReverseString(input)
	case "jwt_decode":
		return executeJWTDecode(input)
	case "json_escape":
		return executeJSONEscape(input)
	case "json_unescape":
		return executeJSONUnescape(input)
	case "unicode_escape":
		return executeUnicodeEscape(input)
	case "unicode_unescape":
		return executeUnicodeUnescape(input)
	case "trim_whitespace":
		return executeTrimWhitespace(input)
	default:
		return "", fmt.Errorf("unknown action type: %s", action.ActionType)
	}
}

// executeBase64Encode encodes the input string to Base64
func executeBase64Encode(input string) (string, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(input))
	return encoded, nil
}

// executeBase64Decode decodes the input string from Base64
func executeBase64Decode(input string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return "", fmt.Errorf("invalid base64 input: %v", err)
	}
	return string(decoded), nil
}

// executeURLEncode URL encodes the input string
func executeURLEncode(input string) (string, error) {
	encoded := url.QueryEscape(input)
	return encoded, nil
}

// executeURLDecode URL decodes the input string
func executeURLDecode(input string) (string, error) {
	decoded, err := url.QueryUnescape(input)
	if err != nil {
		return "", fmt.Errorf("invalid URL encoding: %v", err)
	}
	return decoded, nil
}

// executeHTMLEncode HTML encodes the input string
func executeHTMLEncode(input string) (string, error) {
	encoded := html.EscapeString(input)
	return encoded, nil
}

// executeHTMLDecode HTML decodes the input string
func executeHTMLDecode(input string) (string, error) {
	decoded := html.UnescapeString(input)
	return decoded, nil
}

// executeHexEncode encodes the input string to hexadecimal
func executeHexEncode(input string) (string, error) {
	encoded := hex.EncodeToString([]byte(input))
	return encoded, nil
}

// executeHexDecode decodes the input string from hexadecimal
func executeHexDecode(input string) (string, error) {
	decoded, err := hex.DecodeString(input)
	if err != nil {
		return "", fmt.Errorf("invalid hex input: %v", err)
	}
	return string(decoded), nil
}

// executeMD5Hash generates MD5 hash of the input string
func executeMD5Hash(input string) (string, error) {
	hasher := md5.New()
	hasher.Write([]byte(input))
	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, nil
}

// executeSHA256Hash generates SHA256 hash of the input string
func executeSHA256Hash(input string) (string, error) {
	hasher := sha256.New()
	hasher.Write([]byte(input))
	hash := hex.EncodeToString(hasher.Sum(nil))
	return hash, nil
}

// executeToUppercase converts the input string to uppercase
func executeToUppercase(input string) (string, error) {
	return strings.ToUpper(input), nil
}

// executeToLowercase converts the input string to lowercase
func executeToLowercase(input string) (string, error) {
	return strings.ToLower(input), nil
}

// executeReverseString reverses the input string
func executeReverseString(input string) (string, error) {
	runes := []rune(input)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes), nil
}

// executeJWTDecode decodes a JWT token (payload only, no signature verification)
func executeJWTDecode(input string) (string, error) {
	parts := strings.Split(input, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid JWT format: expected 3 parts separated by dots")
	}

	// Decode the payload (second part)
	payload := parts[1]

	// Add padding if necessary
	if len(payload)%4 != 0 {
		payload += strings.Repeat("=", 4-len(payload)%4)
	}

	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return "", fmt.Errorf("failed to decode JWT payload: %v", err)
	}

	// Pretty print JSON if possible
	var jsonData interface{}
	if err := json.Unmarshal(decoded, &jsonData); err == nil {
		prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
		if err == nil {
			return string(prettyJSON), nil
		}
	}

	return string(decoded), nil
}

// executeJSONEscape escapes a string for use in JSON
func executeJSONEscape(input string) (string, error) {
	escaped, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to JSON escape: %v", err)
	}
	// Remove the surrounding quotes that json.Marshal adds
	result := string(escaped)
	if len(result) >= 2 && result[0] == '"' && result[len(result)-1] == '"' {
		result = result[1 : len(result)-1]
	}
	return result, nil
}

// executeJSONUnescape unescapes a JSON-escaped string
func executeJSONUnescape(input string) (string, error) {
	// Add quotes around the input for JSON unmarshaling
	quotedInput := `"` + input + `"`

	var result string
	err := json.Unmarshal([]byte(quotedInput), &result)
	if err != nil {
		return "", fmt.Errorf("failed to JSON unescape: %v", err)
	}
	return result, nil
}

// executeUnicodeEscape escapes non-ASCII characters to Unicode escape sequences
func executeUnicodeEscape(input string) (string, error) {
	var result strings.Builder
	for _, r := range input {
		if r > 127 {
			result.WriteString(fmt.Sprintf("\\u%04x", r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String(), nil
}

// executeUnicodeUnescape unescapes Unicode escape sequences
func executeUnicodeUnescape(input string) (string, error) {
	var result strings.Builder
	runes := []rune(input)

	for i := 0; i < len(runes); i++ {
		if i < len(runes)-5 && runes[i] == '\\' && runes[i+1] == 'u' {
			// Try to parse the next 4 characters as hex
			hexStr := string(runes[i+2 : i+6])
			if codePoint, err := strconv.ParseInt(hexStr, 16, 32); err == nil {
				result.WriteRune(rune(codePoint))
				i += 5 // Skip the \uXXXX sequence
			} else {
				result.WriteRune(runes[i])
			}
		} else {
			result.WriteRune(runes[i])
		}
	}

	return result.String(), nil
}

// executeTrimWhitespace removes leading and trailing whitespace
func executeTrimWhitespace(input string) (string, error) {
	return strings.TrimSpace(input), nil
}

// GetPreview generates a preview of what the action would do to the input
func GetPreview(action ChefAction, input string) string {
	result, err := executeAction(action, input)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	// Limit preview length to avoid UI issues
	if len(result) > 200 {
		return result[:200] + "..."
	}

	return result
}

// GetAvailableActions returns a list of all available chef actions
func GetAvailableActions() []map[string]string {
	return []map[string]string{
		{
			"id":          "base64_encode",
			"name":        "Base64 Encode",
			"description": "Encode text to Base64",
		},
		{
			"id":          "base64_decode",
			"name":        "Base64 Decode",
			"description": "Decode text from Base64",
		},
		{
			"id":          "url_encode",
			"name":        "URL Encode",
			"description": "URL encode text (percent encoding)",
		},
		{
			"id":          "url_decode",
			"name":        "URL Decode",
			"description": "URL decode text (percent decoding)",
		},
		{
			"id":          "html_encode",
			"name":        "HTML Encode",
			"description": "HTML encode text (escape HTML entities)",
		},
		{
			"id":          "html_decode",
			"name":        "HTML Decode",
			"description": "HTML decode text (unescape HTML entities)",
		},
		{
			"id":          "hex_encode",
			"name":        "Hex Encode",
			"description": "Encode text to hexadecimal",
		},
		{
			"id":          "hex_decode",
			"name":        "Hex Decode",
			"description": "Decode text from hexadecimal",
		},
		{
			"id":          "md5_hash",
			"name":        "MD5 Hash",
			"description": "Generate MD5 hash of the input",
		},
		{
			"id":          "sha256_hash",
			"name":        "SHA256 Hash",
			"description": "Generate SHA256 hash of the input",
		},
		{
			"id":          "to_uppercase",
			"name":        "To Uppercase",
			"description": "Convert text to uppercase",
		},
		{
			"id":          "to_lowercase",
			"name":        "To Lowercase",
			"description": "Convert text to lowercase",
		},
		{
			"id":          "reverse_string",
			"name":        "Reverse String",
			"description": "Reverse the order of characters in the string",
		},
		{
			"id":          "jwt_decode",
			"name":        "JWT Decode",
			"description": "Decode JWT token payload (no signature verification)",
		},
		{
			"id":          "json_escape",
			"name":        "JSON Escape",
			"description": "Escape string for use in JSON",
		},
		{
			"id":          "json_unescape",
			"name":        "JSON Unescape",
			"description": "Unescape JSON-escaped string",
		},
		{
			"id":          "unicode_escape",
			"name":        "Unicode Escape",
			"description": "Escape non-ASCII characters to Unicode sequences",
		},
		{
			"id":          "unicode_unescape",
			"name":        "Unicode Unescape",
			"description": "Unescape Unicode escape sequences",
		},
		{
			"id":          "trim_whitespace",
			"name":        "Trim Whitespace",
			"description": "Remove leading and trailing whitespace",
		},
	}
}

// GetSequentialPreview generates previews for all actions up to the specified index
// This shows the cumulative effect of sequential processing
func GetSequentialPreview(actions []ChefAction, inputValue string, upToIndex int) ([]string, error) {
	if upToIndex < 0 || upToIndex >= len(actions) {
		return nil, fmt.Errorf("invalid index: %d", upToIndex)
	}

	previews := make([]string, upToIndex+1)
	currentValue := inputValue

	// Process actions sequentially up to the specified index
	for i := 0; i <= upToIndex; i++ {
		// Skip actions with empty actionType
		if actions[i].ActionType == "" {
			previews[i] = currentValue // Keep current value unchanged
			continue
		}

		result, err := executeAction(actions[i], currentValue)
		if err != nil {
			return nil, fmt.Errorf("error at action %d: %v", i, err)
		}

		// Limit preview length to avoid UI issues
		if len(result) > 200 {
			previews[i] = result[:200] + "..."
		} else {
			previews[i] = result
		}

		currentValue = result
	}

	return previews, nil
}

// GetAllSequentialPreviews generates previews for all actions in sequence
func GetAllSequentialPreviews(actions []ChefAction, inputValue string) ([]string, error) {
	if len(actions) == 0 {
		return []string{}, nil
	}

	previews := make([]string, len(actions))
	currentValue := inputValue

	// Process actions sequentially
	for i, action := range actions {
		result, err := executeAction(action, currentValue)
		if err != nil {
			return nil, fmt.Errorf("error at action %d: %v", i, err)
		}

		// Limit preview length to avoid UI issues
		if len(result) > 200 {
			previews[i] = result[:200] + "..."
		} else {
			previews[i] = result
		}

		currentValue = result
	}

	return previews, nil
}
