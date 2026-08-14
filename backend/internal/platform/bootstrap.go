package platform

import (
	"errors"
	"os"
)

type BootstrapInput struct {
	Username     string
	Role         string
	PasswordUser bool
	SSOUser      bool
}

func BootstrapAdministrator(input BootstrapInput) (RoleAssignment, error) {
	if NormalizeIdentityKey(input.Username) == "" || input.Role == "" {
		return RoleAssignment{}, errors.New("bootstrap username and role are required")
	}
	if !input.PasswordUser && !input.SSOUser {
		return RoleAssignment{}, errors.New("bootstrap user has no login method")
	}
	return RoleAssignment{UserID: NormalizeIdentityKey(input.Username), RoleID: input.Role, SourceType: "system", SourceKey: "bootstrap"}, nil
}

func BootstrapUsername() string { return NormalizeIdentityKey(os.Getenv("GLYFLOW_BOOTSTRAP_USERNAME")) }
