package api

import (
	"encoding/json"
	"net/http"

	"github.com/VBenevides/Glyphflow/backend/internal/platform"
)

const openAPISpec = `{
  "openapi": "3.0.3",
  "info": {"title": "Glyphflow Control Plane API", "version": "1.0.0"},
  "servers": [{"url": "/"}],
  "components": {
    "securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer", "bearerFormat": "Glyphflow access token"}},
    "schemas": {
      "Credentials": {"type": "object", "required": ["email", "password"], "properties": {"email": {"type": "string", "format": "email"}, "password": {"type": "string", "format": "password"}}},
      "RefreshRequest": {"type": "object", "required": ["session_id", "refresh_token"], "properties": {"session_id": {"type": "string"}, "refresh_token": {"type": "string"}}},
      "SessionRequest": {"type": "object", "required": ["session_id"], "properties": {"session_id": {"type": "string"}}},
      "AuthResponse": {"type": "object", "properties": {"status": {"type": "string"}}},
      "SystemMetrics": {"type": "object", "required": ["generatedAt", "ready", "metrics", "signals", "alerts"], "properties": {"generatedAt": {"type": "string", "format": "date-time"}, "ready": {"type": "boolean"}, "metrics": {"type": "object", "additionalProperties": {"type": "integer"}}, "signals": {"type": "object"}, "alerts": {"type": "array", "items": {"type": "object"}}}},
      "DeadLetter": {"type": "object", "required": ["id", "stream", "consumer", "subject", "messageId", "state"], "properties": {"id": {"type": "string"}, "runnerId": {"type": "string"}, "stream": {"type": "string"}, "consumer": {"type": "string"}, "subject": {"type": "string"}, "messageId": {"type": "string"}, "payloadSha256": {"type": "string"}, "error": {"type": "string"}, "correlationId": {"type": "string"}, "state": {"type": "string"}, "attempts": {"type": "integer"}, "firstFailedAt": {"type": "string", "format": "date-time"}, "lastFailedAt": {"type": "string", "format": "date-time"}}},
      "DeadLetterAction": {"type": "object", "required": ["reason"], "properties": {"reason": {"type": "string", "maxLength": 512}, "state": {"type": "string", "enum": ["RECONCILED", "DISCARDED"]}}},
      "ScheduleProjection": {"type": "object", "required": ["available"], "properties": {"available": {"type": "boolean"}, "calculatedAt": {"type": "string", "format": "date-time"}, "windowStart": {"type": "string", "format": "date-time"}, "windowEnd": {"type": "string", "format": "date-time"}, "durationSource": {"type": "string"}, "segments": {"type": "array", "items": {"type": "object"}}, "conflicts": {"type": "array", "items": {"type": "object"}}}},
      "SecretAttention": {"type": "object", "required": ["id", "name", "status"], "properties": {"id": {"type": "string"}, "name": {"type": "string"}, "status": {"type": "string", "enum": ["UNKNOWN", "VALID", "INTEGRITY_FAILED", "KEY_UNAVAILABLE", "DECRYPTION_FAILED"]}}},
      "Error": {"type": "object", "properties": {"error": {"type": "string"}}}
    }
  },
  "paths": {
    "/api/v1/healthz": {"get": {"tags": ["Health"], "summary": "Health check", "responses": {"200": {"description": "Healthy"}}}},
    "/api/v1/readyz": {"get": {"tags": ["Health"], "summary": "Readiness check", "responses": {"200": {"description": "Ready"}, "503": {"description": "Not ready"}}}},
    "/api/v1/config": {"get": {"tags": ["Health"], "summary": "Get public runtime configuration", "responses": {"200": {"description": "Runtime configuration"}}}},
    "/api/v1/auth/register": {"post": {"tags": ["Authentication"], "summary": "Register with an email and password", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Credentials"}}}}, "responses": {"201": {"description": "Created"}, "400": {"description": "Registration failed"}}}},
    "/api/v1/auth/login": {"post": {"tags": ["Authentication"], "summary": "Login with an email and password", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Credentials"}}}}, "responses": {"200": {"description": "Authenticated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/AuthResponse"}}}}, "401": {"description": "Invalid credentials"}}}},
    "/api/v1/auth/refresh": {"post": {"tags": ["Authentication"], "summary": "Rotate the refresh token", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/RefreshRequest"}}}}, "responses": {"200": {"description": "Refreshed", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/AuthResponse"}}}}, "401": {"description": "Invalid refresh token"}}}},
    "/api/v1/auth/logout": {"post": {"tags": ["Authentication"], "summary": "Logout a session", "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/SessionRequest"}}}}, "responses": {"204": {"description": "Logged out"}}}},
    "/api/v1/auth/logout-all": {"post": {"tags": ["Authentication"], "summary": "Logout all sessions", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Logged out"}}}},
    "/api/v1/auth/oidc/providers": {"get": {"tags": ["Authentication"], "summary": "List enabled OIDC providers", "responses": {"200": {"description": "Providers"}}}},
    "/api/v1/auth/oidc/login": {"get": {"tags": ["Authentication"], "summary": "Start OIDC login", "parameters": [{"name": "provider", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "redirect_uri", "in": "query", "required": true, "schema": {"type": "string", "format": "uri"}}], "responses": {"302": {"description": "Redirect to the identity provider"}}}},
    "/api/v1/auth/oidc/link": {"get": {"tags": ["Authentication"], "summary": "Start authenticated OIDC identity linking", "security": [{"bearerAuth": []}], "parameters": [{"name": "provider", "in": "query", "required": true, "schema": {"type": "string"}}], "responses": {"302": {"description": "Redirect to the identity provider"}}}},
    "/api/v1/auth/oidc/callback": {"get": {"tags": ["Authentication"], "summary": "Complete OIDC login", "responses": {"200": {"description": "Authenticated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/AuthResponse"}}}}, "401": {"description": "OIDC callback failed"}}}},
    "/api/v1/me": {"get": {"tags": ["Identity"], "summary": "Get the current user", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Current user"}}}, "put": {"tags": ["Identity"], "summary": "Update the current profile", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Updated profile"}}}},
    "/api/v1/me/password": {"post": {"tags": ["Identity"], "summary": "Change the current password", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Password changed"}}}},
    "/api/v1/me/identities/{identity_id}": {"delete": {"tags": ["Identity"], "summary": "Unlink an owned OIDC identity", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Identity unlinked"}}}},
    "/api/v1/me/sessions/revoke": {"post": {"tags": ["Identity"], "summary": "Revoke the current session", "security": [{"bearerAuth": []}], "parameters": [{"name": "session_id", "in": "query", "schema": {"type": "string"}}], "responses": {"204": {"description": "Session revoked"}}}},
    "/api/v1/tasks": {"get": {"tags": ["Tasks"], "summary": "List tasks", "security": [{"bearerAuth": []}], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer", "minimum": 1}}], "responses": {"200": {"description": "Task page"}}}, "post": {"tags": ["Tasks"], "summary": "Create a task", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"201": {"description": "Created"}, "400": {"description": "Invalid task"}}}},
    "/api/v1/tasks/{task_id}": {"get": {"tags": ["Tasks"], "summary": "Get a task", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Task"}, "404": {"description": "Not found"}}}, "delete": {"tags": ["Tasks"], "summary": "Archive a task", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Archived"}, "404": {"description": "Not found"}}}},
    "/api/v1/global-variables/options": {"get": {"tags": ["Operations"], "summary": "List global variable options", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Global variable options"}}}},
    "/api/v1/global-variables": {"get": {"tags": ["Operations"], "summary": "List global variables", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Global variable page"}}}, "post": {"tags": ["Operations"], "summary": "Create a global variable", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Created"}}}},
    "/api/v1/global-variables/{variable_id}": {"get": {"tags": ["Operations"], "summary": "Get a global variable", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Global variable"}}}, "put": {"tags": ["Operations"], "summary": "Update a global variable", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Updated"}}}, "delete": {"tags": ["Operations"], "summary": "Delete a global variable", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Deleted"}}}},
    "/api/v1/schedules": {"get": {"tags": ["Operations"], "summary": "List schedules", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Schedule page"}}}, "post": {"tags": ["Operations"], "summary": "Create a schedule", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"201": {"description": "Created"}, "400": {"description": "Invalid schedule"}, "409": {"description": "Exclusive-resource conflict"}}}},
    "/api/v1/schedule-projection": {"get": {"tags": ["Operations"], "summary": "Get the latest seven-day schedule projection", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Latest projection snapshot", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/ScheduleProjection"}}}}}}},
    "/api/v1/schedules/preview": {"post": {"tags": ["Operations"], "summary": "Preview schedule occurrences", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Occurrences"}}}},
    "/api/v1/schedules/{schedule_id}": {"get": {"tags": ["Operations"], "summary": "Get a schedule", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Schedule"}}}, "post": {"tags": ["Operations"], "summary": "Update a schedule", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Updated schedule"}}}, "delete": {"tags": ["Operations"], "summary": "Permanently delete a schedule", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Deleted"}, "404": {"description": "Not found"}, "409": {"description": "Schedule is referenced by execution history"}}}},
    "/api/v1/resources": {"get": {"tags": ["Operations"], "summary": "List resources", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Resource page"}}}, "post": {"tags": ["Operations"], "summary": "Create a resource", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}, "kind": {"type": "string", "enum": ["exclusive", "non-blocking"], "default": "exclusive"}}}}}}, "responses": {"201": {"description": "Resource created"}, "400": {"description": "Invalid resource"}}}},
    "/api/v1/resources/{resource_id}": {"get": {"tags": ["Operations"], "summary": "Get a resource", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Resource"}, "404": {"description": "Not found"}}}, "delete": {"tags": ["Operations"], "summary": "Delete a resource", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Deleted"}, "409": {"description": "Resource is in use"}}}},
    "/api/v1/resources/{resource_id}/lease": {"post": {"tags": ["Operations"], "summary": "Acquire a resource lease", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Lease"}, "409": {"description": "Lease conflict"}}}, "delete": {"tags": ["Operations"], "summary": "Release a resource lease", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Released"}}}},
    "/api/v1/users": {"get": {"tags": ["Administration"], "summary": "List users", "security": [{"bearerAuth": []}], "parameters": [{"name": "email", "in": "query", "schema": {"type": "string"}}, {"name": "status", "in": "query", "schema": {"type": "string", "enum": ["active", "pending", "disabled"]}}, {"name": "roles", "in": "query", "description": "Comma-separated roles; users must have every listed role", "schema": {"type": "string"}}, {"name": "page", "in": "query", "schema": {"type": "integer"}}, {"name": "limit", "in": "query", "schema": {"type": "integer"}}], "responses": {"200": {"description": "User page"}}}, "post": {"tags": ["Administration"], "summary": "Create a user", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Created"}}}},
    "/api/v1/users/{user_id}": {"get": {"tags": ["Administration"], "summary": "Get user details", "security": [{"bearerAuth": []}], "parameters": [{"name": "user_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "User details"}, "403": {"description": "Forbidden"}, "404": {"description": "Not found"}}}},
    "/api/v1/roles": {"get": {"deprecated": true, "tags": ["Administration"], "summary": "Deprecated role listing", "description": "Use /api/v1/admin/roles.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}, "post": {"deprecated": true, "tags": ["Administration"], "summary": "Deprecated role management", "description": "Use /api/v1/admin/roles.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/sso": {"get": {"deprecated": true, "tags": ["Administration"], "summary": "Deprecated SSO configuration", "description": "Use /api/v1/admin/auth/providers.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}, "post": {"deprecated": true, "tags": ["Administration"], "summary": "Deprecated SSO configuration", "description": "Use /api/v1/admin/auth/providers.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/logs": {"get": {"deprecated": true, "tags": ["Operations"], "summary": "Deprecated log listing", "description": "Use /api/v1/runs/{run_id}/logs.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/runs": {"get": {"tags": ["Runs"], "summary": "List runs", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Run page"}}}},
    "/api/v1/runs/{run_id}": {"get": {"tags": ["Runs"], "summary": "Get a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Run"}}}},
    "/api/v1/runs/{run_id}/cancel": {"post": {"tags": ["Runs"], "summary": "Cancel a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Cancelled"}}}},
    "/api/v1/runs/{run_id}/retry": {"post": {"tags": ["Runs"], "summary": "Retry a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Retried"}}}},
    "/api/v1/runs/{run_id}/reconcile": {"post": {"tags": ["Runs"], "summary": "Reconcile an unknown run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Reconciled"}}}},
    "/api/v1/runs/{run_id}/events": {"get": {"tags": ["Runs"], "summary": "List run events", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Events"}}}},
    "/api/v1/runs/{run_id}/logs": {"get": {"tags": ["Runs"], "summary": "Stream run logs", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Log chunks"}}}},
    "/api/v1/runs/{run_id}/logs/download": {"get": {"tags": ["Runs"], "summary": "Download run logs", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Log download"}}}},
    "/api/v1/events": {"get": {"deprecated": true, "tags": ["Runs"], "summary": "Deprecated event listing", "description": "Use /api/v1/runs/{run_id}/events.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/runners": {"get": {"tags": ["Runners"], "summary": "List runners", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner page"}}}},
    "/api/v1/runners/pools": {"get": {"tags": ["Runners"], "summary": "List runner pools", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner pool page"}}}, "post": {"tags": ["Runners"], "summary": "Create a runner pool", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}, "description": {"type": "string"}, "enabled": {"type": "boolean"}}}}}}, "responses": {"201": {"description": "Runner pool created"}, "409": {"description": "Pool name already exists"}}}},
    "/api/v1/runners/pools/{pool_id}": {"get": {"tags": ["Runners"], "summary": "Get a runner pool", "security": [{"bearerAuth": []}], "parameters": [{"name": "pool_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "Runner pool"}, "404": {"description": "Not found"}}}, "put": {"tags": ["Runners"], "summary": "Update a runner pool", "security": [{"bearerAuth": []}], "parameters": [{"name": "pool_id", "in": "path", "required": true, "schema": {"type": "string"}}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name"], "properties": {"name": {"type": "string"}, "description": {"type": "string"}, "enabled": {"type": "boolean"}}}}}}, "responses": {"200": {"description": "Runner pool updated"}, "409": {"description": "Pool name already exists"}}}, "delete": {"tags": ["Runners"], "summary": "Archive a runner pool", "security": [{"bearerAuth": []}], "parameters": [{"name": "pool_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Archived"}, "409": {"description": "Pool is still in use"}}}},
    "/api/v1/runners/{runner_id}": {"get": {"tags": ["Runners"], "summary": "Get a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner"}}}, "put": {"tags": ["Runners"], "summary": "Update runner capacity", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["capacity"], "properties": {"capacity": {"type": "integer", "minimum": 1}}}}}}, "responses": {"200": {"description": "Runner updated"}, "400": {"description": "Invalid capacity"}, "404": {"description": "Not found"}}}, "delete": {"tags": ["Runners"], "summary": "Archive a runner permanently", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Archived"}, "404": {"description": "Not found"}}}},
    "/api/v1/runners/{runner_id}/drain": {"post": {"tags": ["Runners"], "summary": "Drain a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner updated"}}}},
    "/api/v1/runners/{runner_id}/revoke": {"post": {"tags": ["Runners"], "summary": "Revoke a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner updated"}}}},
    "/api/v1/runners/enrollments": {"post": {"tags": ["Runners"], "summary": "Create a one-use runner binary enrollment", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Executable enrollment artifact"}, "400": {"description": "Unsupported platform"}, "503": {"description": "Runner binaries unavailable"}}}},
    "/api/v1/runners/enroll": {"post": {"tags": ["Runners"], "summary": "Consume a one-use runner enrollment", "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["runner_id", "token"]}}}}, "responses": {"200": {"description": "Runner connection"}, "401": {"description": "Invalid or used enrollment"}}}},
    "/api/v1/audit": {"get": {"tags": ["Administration"], "summary": "List audit events", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Audit page"}}}},
    "/api/v1/admin/system/metrics": {"get": {"tags": ["Administration"], "summary": "Get operational system metrics", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "System metrics", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/SystemMetrics"}}}}, "503": {"description": "Metrics unavailable"}}}},
    "/api/v1/admin/secrets/attention": {"get": {"tags": ["Administration"], "summary": "List secrets requiring attention", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Secret integrity statuses", "content": {"application/json": {"schema": {"type": "array", "items": {"$ref": "#/components/schemas/SecretAttention"}}}}}, "503": {"description": "Secret status unavailable"}}}},
    "/api/v1/admin/secrets": {"get": {"tags": ["Administration"], "summary": "List named secrets without values", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Secret metadata"}}}, "post": {"tags": ["Administration"], "summary": "Create an encrypted named secret", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name", "secret_value"], "properties": {"name": {"type": "string"}, "secret_value": {"type": "string", "format": "password", "writeOnly": true}}}}}}, "responses": {"201": {"description": "Secret created"}}}},
    "/api/v1/admin/secrets/{secret_id}": {"put": {"tags": ["Administration"], "summary": "Replace an encrypted named secret", "security": [{"bearerAuth": []}], "parameters": [{"name": "secret_id", "in": "path", "required": true, "schema": {"type": "string"}}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["name", "secret_value"], "properties": {"name": {"type": "string"}, "secret_value": {"type": "string", "format": "password", "writeOnly": true}}}}}}, "responses": {"200": {"description": "Secret replaced"}}}},
    "/api/v1/admin/dead-letters": {"get": {"tags": ["Administration"], "summary": "List dead-letter records", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Dead-letter page", "content": {"application/json": {"schema": {"type": "object"}}}}, "503": {"description": "Dead-letter storage unavailable"}}}},
    "/api/v1/admin/dead-letters/{dead_letter_id}": {"get": {"tags": ["Administration"], "summary": "Inspect a dead-letter record", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Dead-letter record", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DeadLetter"}}}}, "404": {"description": "Not found"}}}},
    "/api/v1/admin/dead-letters/{dead_letter_id}/retry": {"post": {"tags": ["Administration"], "summary": "Retry a dead-letter record", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DeadLetterAction"}}}}, "responses": {"202": {"description": "Retry queued"}, "409": {"description": "Record is not actionable"}}}},
    "/api/v1/admin/dead-letters/{dead_letter_id}/reconcile": {"post": {"tags": ["Administration"], "summary": "Terminally reconcile a dead-letter record", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/DeadLetterAction"}}}}, "responses": {"200": {"description": "Reconciled"}, "409": {"description": "Record is not actionable"}}}},
    "/api/v1/runs/execute": {"post": {"tags": ["Runs"], "summary": "Execute a task", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["task_id"]}}}}, "responses": {"201": {"description": "Run created"}, "400": {"description": "Invalid task request"}, "503": {"description": "Run storage unavailable"}}}},
    "/api/v1/runs/retry": {"post": {"deprecated": true, "tags": ["Runs"], "summary": "Deprecated run retry", "description": "Use /api/v1/runs/{run_id}/retry.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/runs/cancel": {"post": {"deprecated": true, "tags": ["Runs"], "summary": "Deprecated run cancellation", "description": "Use /api/v1/runs/{run_id}/cancel.", "security": [{"bearerAuth": []}], "responses": {"410": {"description": "Gone"}}}},
    "/api/v1/admin/auth/settings": {"post": {"tags": ["Administration"], "summary": "Update password authentication settings", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"200": {"description": "Settings updated"}}}},
    "/api/v1/admin/execution-status": {"get": {"tags": ["Administration"], "summary": "List exit code meanings", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Exit code meanings"}}}, "post": {"tags": ["Administration"], "summary": "Create an exit code meaning", "security": [{"bearerAuth": []}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["code", "meaning"], "properties": {"code": {"type": "integer"}, "meaning": {"type": "string"}}}}}}, "responses": {"201": {"description": "Created"}}}},
    "/api/v1/admin/execution-status/{code}": {"put": {"tags": ["Administration"], "summary": "Update a custom exit code", "security": [{"bearerAuth": []}], "parameters": [{"name": "code", "in": "path", "required": true, "schema": {"type": "integer"}}], "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "required": ["meaning"], "properties": {"code": {"type": "integer"}, "meaning": {"type": "string"}}}}}}, "responses": {"200": {"description": "Updated"}, "409": {"description": "System exit codes cannot be changed"}}}, "delete": {"tags": ["Administration"], "summary": "Delete a custom exit code", "security": [{"bearerAuth": []}], "parameters": [{"name": "code", "in": "path", "required": true, "schema": {"type": "integer"}}], "responses": {"204": {"description": "Deleted"}, "409": {"description": "System exit codes cannot be deleted"}}}},
    "/api/v1/admin/auth/sessions/revoke": {"post": {"tags": ["Administration"], "summary": "Revoke a session", "security": [{"bearerAuth": []}], "parameters": [{"name": "session_id", "in": "query", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Session revoked"}}}},
    "/api/v1/admin/auth/sessions": {"get": {"tags": ["Administration"], "summary": "List active sessions", "security": [{"bearerAuth": []}], "parameters": [{"name": "email", "in": "query", "schema": {"type": "string"}}, {"name": "page", "in": "query", "schema": {"type": "integer"}}, {"name": "limit", "in": "query", "schema": {"type": "integer"}}], "responses": {"200": {"description": "Session page"}}}},
    "/api/v1/admin/auth/providers": {"get": {"tags": ["Administration"], "summary": "List OIDC providers", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Providers"}}}, "post": {"tags": ["Administration"], "summary": "Add or update an OIDC provider", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {"clientSecret": {"type": "string", "format": "password", "writeOnly": true}}}}}}, "responses": {"201": {"description": "Provider updated"}}}},
    "/api/v1/admin/auth/users/{user_id}/disable": {"post": {"tags": ["Administration"], "summary": "Disable a user", "security": [{"bearerAuth": []}], "parameters": [{"name": "user_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "User disabled"}}}},
    "/api/v1/admin/auth/users/{user_id}/sessions/revoke-all": {"post": {"tags": ["Administration"], "summary": "Revoke all user sessions", "security": [{"bearerAuth": []}], "parameters": [{"name": "user_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Sessions revoked"}}}},
    "/api/v1/admin/roles": {"get": {"tags": ["Administration"], "summary": "List managed roles", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Roles"}}}, "post": {"tags": ["Administration"], "summary": "Create a role", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"201": {"description": "Role created"}}}},
    "/api/v1/admin/roles/{role_id}": {"put": {"tags": ["Administration"], "summary": "Replace role permissions", "security": [{"bearerAuth": []}], "parameters": [{"name": "role_id", "in": "path", "required": true, "schema": {"type": "string"}}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"200": {"description": "Role updated"}}}, "delete": {"tags": ["Administration"], "summary": "Delete a role", "security": [{"bearerAuth": []}], "parameters": [{"name": "role_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Role deleted"}}}}
  }
}`

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Glyphflow API Docs</title>
</head>
<body>
  <main>
    <section style="max-width: 1460px; margin: 16px auto; padding: 16px; border: 1px solid #ddd; border-radius: 4px;">
      <form id="login-form">
        <strong>Authorize with email and password</strong>
        <input id="email" name="email" type="text" inputmode="email" autocomplete="email" placeholder="Email" required>
        <input id="password" name="password" type="password" autocomplete="current-password" placeholder="Password" required>
        <button type="submit">Authorize</button>
        <span id="login-status" role="status"></span>
      </form>
    </section>
    <section>
      <h1>Offline OpenAPI specification</h1>
      <p><a href="/openapi.json">OpenAPI JSON</a></p>
      <pre id="openapi-spec">Loading…</pre>
    </section>
  </main>
  <script>
    fetch('/openapi.json').then(function (response) {
      if (!response.ok) throw new Error('OpenAPI document unavailable');
      return response.json();
    }).then(function (spec) {
      document.getElementById('openapi-spec').textContent = JSON.stringify(spec, null, 2);
    }).catch(function (error) {
      document.getElementById('openapi-spec').textContent = error.message;
    });
    document.getElementById('login-form').addEventListener('submit', async function (event) {
      event.preventDefault();
      const status = document.getElementById('login-status');
      status.textContent = 'Signing in…';
      try {
        const response = await fetch('/docs/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email: document.getElementById('email').value, password: document.getElementById('password').value})});
        const body = await response.json();
        if (!response.ok) throw new Error(body.error || 'Login failed');
        status.textContent = 'Authorized';
      } catch (error) {
        status.textContent = error.message;
      }
    });
  </script>
</body>
</html>`

func swaggerUI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(swaggerHTML))
}

func openAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(openAPISpec))
}

func (s Server) docsLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var in passwordRequest
	if json.NewDecoder(r.Body).Decode(&in) != nil || s.AuthService == nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if !s.allowAuth(w, r, "password-login|"+platform.NormalizeIdentityKey(in.Email)) {
		return
	}
	tokens, err := s.AuthService.Login(in.Email, in.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	s.setSessionCookies(w, tokens)
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}
