package platform

import (
	"errors"
	"strings"
)

func HasLoginMethod(passwordEnabled bool, enabledSSO int) bool {
	return passwordEnabled || enabledSSO > 0
}

func ValidateLoginMethodRemoval(passwordEnabled bool, enabledSSO int, removePassword bool) error {
	if removePassword && !HasLoginMethod(false, enabledSSO) {
		return errors.New("user would have no login method")
	}
	return nil
}

func NormalizeIdentityKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
