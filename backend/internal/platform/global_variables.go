package platform

import (
	"errors"
	"regexp"
	"strings"
)

var globalVariablePattern = regexp.MustCompile(`\$ENV:([A-Z_][A-Z0-9_]*)`)
var globalVariableNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ResolveGlobalVariables expands the supported $ENV:VAR_NAME form once.
func ResolveGlobalVariables(value string, variables map[string]string) (string, error) {
	var missing string
	resolved := globalVariablePattern.ReplaceAllStringFunc(value, func(match string) string {
		name := globalVariablePattern.FindStringSubmatch(match)[1]
		if replacement, ok := variables[name]; ok {
			return replacement
		}
		missing = name
		return match
	})
	if missing != "" {
		return "", errors.New("global variable is not defined: " + missing)
	}
	return resolved, nil
}

func ResolveEnvironment(values, variables map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(values))
	for name, value := range values {
		resolvedName, err := ResolveGlobalVariables(name, variables)
		if err != nil {
			return nil, err
		}
		resolvedValue, err := ResolveGlobalVariables(value, variables)
		if err != nil {
			return nil, err
		}
		resolved[resolvedName] = resolvedValue
	}
	return resolved, nil
}

func GlobalVariableName(value string) bool {
	return globalVariableNamePattern.MatchString(strings.TrimSpace(value))
}
