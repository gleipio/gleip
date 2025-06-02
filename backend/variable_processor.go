package backend

import (
	"fmt"
	"regexp"
	"strings"
)

// DefaultVariableProcessor implements the VariableProcessor interface
type DefaultVariableProcessor struct{}

// NewVariableProcessor creates a new variable processor
func NewVariableProcessor() VariableProcessor {
	return &DefaultVariableProcessor{}
}

// ProcessVariables replaces variables in a string with their values
func (p *DefaultVariableProcessor) ProcessVariables(input string, variables map[string]string) string {
	result := input

	// First check if we actually have any variable patterns in the input
	// Variable pattern is anything enclosed in {{ }}
	varPattern := regexp.MustCompile(`\{\{([^}]+)\}\}`)
	potentialMatches := varPattern.FindAllStringSubmatch(input, -1)

	if len(potentialMatches) == 0 {
		return result
	}

	replacementCount := 0

	// First replace variables with defaults
	defaultVarPattern := regexp.MustCompile(`\{\{([^:}]+):([^}]*)\}\}`)
	defaultMatches := defaultVarPattern.FindAllStringSubmatch(input, -1)
	for _, match := range defaultMatches {
		if len(match) > 2 {
			varName := strings.TrimSpace(match[1])
			varDefault := match[2]
			fullPattern := match[0]

			// Only use default if variable doesn't exist
			if value, exists := variables[varName]; exists {
				result = strings.ReplaceAll(result, fullPattern, value)
			} else {
				result = strings.ReplaceAll(result, fullPattern, varDefault)
			}
			replacementCount++
		}
	}

	// Then replace standard variables
	for name, value := range variables {
		varPattern := fmt.Sprintf("{{%s}}", name)
		if strings.Contains(result, varPattern) {
			result = strings.ReplaceAll(result, varPattern, value)
			replacementCount++
		}
	}

	return result
}
