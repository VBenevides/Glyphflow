package platform

import (
	"errors"
	"strings"
)

type SSOIdentity struct {
	UserID   string
	Provider string
	Subject  string
}

func MatchSSOIdentity(identities []SSOIdentity, provider, subject string) (string, bool) {
	provider, subject = NormalizeIdentityKey(provider), strings.TrimSpace(subject)
	if provider == "" || subject == "" {
		return "", false
	}
	for _, identity := range identities {
		if NormalizeIdentityKey(identity.Provider) == provider && identity.Subject == subject {
			return identity.UserID, true
		}
	}
	return "", false
}

func CanLinkSSOIdentity(authenticatedUserID, targetUserID string) error {
	if authenticatedUserID == "" || targetUserID == "" || authenticatedUserID != targetUserID {
		return errors.New("authenticated account linking is required")
	}
	return nil
}

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
