package api

import (
	"errors"
	"net/http"
)

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

type routeRegistrar interface {
	Handle(string, http.Handler)
	HandleFunc(string, http.HandlerFunc)
}

type trackedMux struct {
	*http.ServeMux
	patterns map[string]struct{}
}

func newTrackedMux() *trackedMux {
	return &trackedMux{ServeMux: http.NewServeMux(), patterns: map[string]struct{}{}}
}

func (m *trackedMux) Handle(pattern string, handler http.Handler) {
	m.patterns[pattern] = struct{}{}
	m.ServeMux.Handle(pattern, handler)
}

func (m *trackedMux) HandleFunc(pattern string, handler http.HandlerFunc) {
	m.patterns[pattern] = struct{}{}
	m.ServeMux.HandleFunc(pattern, handler)
}

var routeDefinitions = []RouteDefinition{
	{Pattern: "/docs", Access: RoutePublic},
	{Pattern: "/docs/login", Access: RoutePublic},
	{Pattern: "/openapi.json", Access: RoutePublic},
	{Pattern: "/api/v1/healthz", Access: RoutePublic},
	{Pattern: "/api/v1/readyz", Access: RoutePublic},
	{Pattern: "/api/v1/config", Access: RoutePublic},
	{Pattern: "/api/v1/runners/enroll", Access: RoutePublic},
	{Pattern: "/api/v1/auth/login", Access: RoutePublic},
	{Pattern: "/api/v1/auth/register", Access: RoutePublic},
	{Pattern: "/api/v1/auth/refresh", Access: RoutePublic},
	{Pattern: "/api/v1/auth/logout", Access: RoutePublic},
	{Pattern: "/api/v1/auth/logout-all", Access: RouteAuthenticated},
	{Pattern: "/api/v1/auth/oidc/providers", Access: RoutePublic},
	{Pattern: "/api/v1/auth/oidc/login", Access: RoutePublic},
	{Pattern: "/api/v1/auth/oidc/callback", Access: RoutePublic},
	{Pattern: "/api/v1/me", Access: RouteAuthenticated},
	{Pattern: "/api/v1/me/password", Access: RouteAuthenticated},
	{Pattern: "/api/v1/me/identities/", Access: RouteAuthenticated},
	{Pattern: "/api/v1/me/sessions/revoke", Access: RouteAuthenticated},
	{Pattern: "/api/v1/tasks", Access: RoutePermission, Permission: "tasks.read|tasks.manage"},
	{Pattern: "/api/v1/schedules", Access: RoutePermission, Permission: "tasks.read|tasks.manage"},
	{Pattern: "/api/v1/schedules/preview", Access: RoutePermission, Permission: "tasks.manage"},
	{Pattern: "/api/v1/schedules/", Access: RoutePermission, Permission: "tasks.read|tasks.manage"},
	{Pattern: "/api/v1/tasks/", Access: RoutePermission, Permission: "runs.cancel|runs.retry"},
	{Pattern: "/api/v1/runs", Access: RoutePermission, Permission: "runs.read"},
	{Pattern: "/api/v1/runs/execute", Access: RoutePermission, Permission: "runs.execute"},
	{Pattern: "/api/v1/runs/retry", Access: RoutePermission, Permission: "runs.retry"},
	{Pattern: "/api/v1/runs/cancel", Access: RoutePermission, Permission: "runs.cancel"},
	{Pattern: "/api/v1/events", Access: RoutePermission, Permission: "logs.read"},
	{Pattern: "/api/v1/logs", Access: RoutePermission, Permission: "logs.read"},
	{Pattern: "/api/v1/resources", Access: RoutePermission, Permission: "resources.read|resources.manage"},
	{Pattern: "/api/v1/resources/", Access: RoutePermission, Permission: "resources.read|resources.manage"},
	{Pattern: "/api/v1/runners", Access: RoutePermission, Permission: "runners.read"},
	{Pattern: "/api/v1/runners/pools", Access: RoutePermission, Permission: "runners.read|runners.manage"},
	{Pattern: "/api/v1/runners/pools/", Access: RoutePermission, Permission: "runners.read|runners.manage"},
	{Pattern: "/api/v1/runners/", Access: RoutePermission, Permission: "runners.read|runners.manage"},
	{Pattern: "/api/v1/runs/", Access: RoutePermission, Permission: "runs.read|runs.cancel|runs.retry|logs.read"},
	{Pattern: "/api/v1/users", Access: RoutePermission, Permission: "users.read|users.manage"},
	{Pattern: "/api/v1/users/", Access: RoutePermission, Permission: "users.read|users.manage"},
	{Pattern: "/api/v1/roles", Access: RoutePermission, Permission: "roles.read|roles.manage"},
	{Pattern: "/api/v1/sso", Access: RoutePermission, Permission: "sso.read|sso.manage"},
	{Pattern: "/api/v1/audit", Access: RoutePermission, Permission: "audit.read"},
	{Pattern: "/api/v1/admin/auth/settings", Access: RoutePermission, Permission: "auth.settings.manage"},
	{Pattern: "/api/v1/admin/auth/sessions/revoke", Access: RoutePermission, Permission: "users.manage"},
	{Pattern: "/api/v1/admin/auth/providers", Access: RoutePermission, Permission: "sso.manage"},
	{Pattern: "/api/v1/admin/auth/users/", Access: RoutePermission, Permission: "users.manage"},
	{Pattern: "/api/v1/admin/roles", Access: RoutePermission, Permission: "roles.manage"},
	{Pattern: "/api/v1/admin/roles/", Access: RoutePermission, Permission: "roles.manage"},
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

func ValidateBuiltRoutes(patterns map[string]struct{}, definitions []RouteDefinition) error {
	registered := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		registered[definition.Pattern] = struct{}{}
	}
	for pattern := range patterns {
		if _, ok := registered[pattern]; !ok {
			return errors.New("built route is unclassified: " + pattern)
		}
	}
	return nil
}
