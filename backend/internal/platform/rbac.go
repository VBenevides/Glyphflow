package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

var PermissionCatalog = []string{
	"users.read", "users.manage", "roles.read", "roles.manage", "sso.read", "sso.manage",
	"auth.settings.manage", "tasks.read", "tasks.manage", "runs.read", "runs.execute", "runs.cancel",
	"runs.retry", "logs.read", "resources.read", "resources.manage", "runners.read", "runners.manage", "audit.read",
}

type SeedRole struct {
	Key         string
	ID          string
	Permissions []string
	System      bool
}

func StableID(key string) string {
	digest := sha256.Sum256([]byte("glyphflow:" + key))
	return hex.EncodeToString(digest[:16])
}

func SeedRoles() ([]SeedRole, error) {
	if len(PermissionCatalog) == 0 {
		return nil, errors.New("permission catalog is empty")
	}
	return []SeedRole{
		{Key: "admin", ID: StableID("role:admin"), Permissions: append([]string(nil), PermissionCatalog...), System: true},
		{Key: "user", ID: StableID("role:user"), Permissions: nil, System: true},
	}, nil
}
