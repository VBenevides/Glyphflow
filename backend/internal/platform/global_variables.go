package platform

import (
	"errors"
	"regexp"
	"strings"
)

var globalVariablePattern = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_]*)\)`)

// ResolveGlobalVariables expands the supported $(VAR_NAME) form once.
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

func GlobalVariableName(value string) bool {
	return strings.TrimSpace(value) != "" && regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(strings.TrimSpace(value))
}
