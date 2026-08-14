package api

import "errors"

type RouteAccess string

const (
	RoutePublic        RouteAccess = "public"
	RouteAuthenticated RouteAccess = "authenticated"
	RoutePermission    RouteAccess = "permission"
)

type RouteDefinition struct {
	Pattern    string
	Access     RouteAccess
	Permission string
}

var routeDefinitions = []RouteDefinition{
	{Pattern: "/api/v1/healthz", Access: RoutePublic},
	{Pattern: "/api/v1/readyz", Access: RoutePublic},
	{Pattern: "/api/v1/auth/login", Access: RoutePublic},
	{Pattern: "/api/v1/auth/register", Access: RoutePublic},
	{Pattern: "/api/v1/auth/refresh", Access: RoutePublic},
	{Pattern: "/api/v1/auth/logout", Access: RoutePublic},
	{Pattern: "/api/v1/auth/logout-all", Access: RouteAuthenticated},
	{Pattern: "/api/v1/auth/oidc/providers", Access: RoutePublic},
	{Pattern: "/api/v1/auth/oidc/login", Access: RoutePublic},
	{Pattern: "/api/v1/me", Access: RouteAuthenticated},
	{Pattern: "/api/v1/me/sessions/revoke", Access: RouteAuthenticated},
	{Pattern: "/api/v1/tasks", Access: RoutePermission, Permission: "tasks.read|tasks.manage"},
	{Pattern: "/api/v1/tasks/", Access: RoutePermission, Permission: "runs.cancel|runs.retry"},
	{Pattern: "/api/v1/runs", Access: RoutePermission, Permission: "runs.read"},
	{Pattern: "/api/v1/events", Access: RoutePermission, Permission: "logs.read"},
	{Pattern: "/api/v1/runners", Access: RoutePermission, Permission: "runners.read"},
	{Pattern: "/api/v1/audit", Access: RoutePermission, Permission: "audit.read"},
	{Pattern: "/api/v1/admin/auth/settings", Access: RoutePermission, Permission: "auth.settings.manage"},
	{Pattern: "/api/v1/admin/auth/sessions/revoke", Access: RoutePermission, Permission: "users.manage"},
	{Pattern: "/api/v1/admin/roles", Access: RoutePermission, Permission: "roles.manage"},
}

func ValidateRouteRegistry(definitions []RouteDefinition) error {
	if len(definitions) == 0 {
		return errors.New("route registry is empty")
	}
	seen := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		if definition.Pattern == "" {
			return errors.New("route pattern is empty")
		}
		if _, exists := seen[definition.Pattern]; exists {
			return errors.New("duplicate route pattern: " + definition.Pattern)
		}
		seen[definition.Pattern] = struct{}{}
		switch definition.Access {
		case RoutePublic, RouteAuthenticated:
			if definition.Permission != "" {
				return errors.New("non-permission route has a permission: " + definition.Pattern)
			}
		case RoutePermission:
			if definition.Permission == "" {
				return errors.New("permission route is unclassified: " + definition.Pattern)
			}
		default:
			return errors.New("route has unknown access: " + definition.Pattern)
		}
	}
	return nil
}

func RouteRegistry() []RouteDefinition {
	return append([]RouteDefinition(nil), routeDefinitions...)
}
