package chef

import (
	"encoding/base64"
	"fmt"
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
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	InputVariable  string       `json:"inputVariable"`  // Name of the input variable
	Actions        []ChefAction `json:"actions"`        // List of transformation actions
	OutputVariable string       `json:"outputVariable"` // Name of the output variable
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
