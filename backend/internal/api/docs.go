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
      "TokenResponse": {"type": "object", "required": ["access_token", "refresh_token", "session_id"], "properties": {"access_token": {"type": "string"}, "refresh_token": {"type": "string"}, "session_id": {"type": "string"}}},
      "Error": {"type": "object", "properties": {"error": {"type": "string"}}}
    }
  },
  "paths": {
    "/api/v1/healthz": {"get": {"tags": ["Health"], "summary": "Health check", "responses": {"200": {"description": "Healthy"}}}},
    "/api/v1/readyz": {"get": {"tags": ["Health"], "summary": "Readiness check", "responses": {"200": {"description": "Ready"}, "503": {"description": "Not ready"}}}},
    "/api/v1/config": {"get": {"tags": ["Health"], "summary": "Get public runtime configuration", "responses": {"200": {"description": "Runtime configuration"}}}},
    "/api/v1/auth/register": {"post": {"tags": ["Authentication"], "summary": "Register with an email and password", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Credentials"}}}}, "responses": {"201": {"description": "Created"}, "400": {"description": "Registration failed"}}}},
    "/api/v1/auth/login": {"post": {"tags": ["Authentication"], "summary": "Login with an email and password", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Credentials"}}}}, "responses": {"200": {"description": "Tokens", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TokenResponse"}}}}, "401": {"description": "Invalid credentials"}}}},
    "/api/v1/auth/refresh": {"post": {"tags": ["Authentication"], "summary": "Rotate the refresh token", "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/RefreshRequest"}}}}, "responses": {"200": {"description": "Tokens", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TokenResponse"}}}}, "401": {"description": "Invalid refresh token"}}}},
    "/api/v1/auth/logout": {"post": {"tags": ["Authentication"], "summary": "Logout a session", "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/SessionRequest"}}}}, "responses": {"204": {"description": "Logged out"}}}},
    "/api/v1/auth/logout-all": {"post": {"tags": ["Authentication"], "summary": "Logout all sessions", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Logged out"}}}},
    "/api/v1/auth/oidc/providers": {"get": {"tags": ["Authentication"], "summary": "List enabled OIDC providers", "responses": {"200": {"description": "Providers"}}}},
    "/api/v1/auth/oidc/login": {"get": {"tags": ["Authentication"], "summary": "Start OIDC login", "parameters": [{"name": "provider", "in": "query", "required": true, "schema": {"type": "string"}}, {"name": "redirect_uri", "in": "query", "required": true, "schema": {"type": "string", "format": "uri"}}], "responses": {"302": {"description": "Redirect to the identity provider"}}}},
    "/api/v1/auth/oidc/callback": {"get": {"tags": ["Authentication"], "summary": "Complete OIDC login", "responses": {"200": {"description": "Tokens", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/TokenResponse"}}}}, "401": {"description": "OIDC callback failed"}}}},
    "/api/v1/me": {"get": {"tags": ["Identity"], "summary": "Get the current user", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Current user"}}}, "put": {"tags": ["Identity"], "summary": "Update the current profile", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Updated profile"}}}},
    "/api/v1/me/password": {"post": {"tags": ["Identity"], "summary": "Change the current password", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Password changed"}}}},
    "/api/v1/me/identities/{identity_id}": {"delete": {"tags": ["Identity"], "summary": "Unlink an owned OIDC identity", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Identity unlinked"}}}},
    "/api/v1/me/sessions/revoke": {"post": {"tags": ["Identity"], "summary": "Revoke the current session", "security": [{"bearerAuth": []}], "parameters": [{"name": "session_id", "in": "query", "schema": {"type": "string"}}], "responses": {"204": {"description": "Session revoked"}}}},
    "/api/v1/tasks": {"get": {"tags": ["Tasks"], "summary": "List tasks", "security": [{"bearerAuth": []}], "parameters": [{"name": "page", "in": "query", "schema": {"type": "integer", "minimum": 1}}], "responses": {"200": {"description": "Task page"}}}, "post": {"tags": ["Tasks"], "summary": "Create a task", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/tasks/{task_id}/{action}": {"post": {"tags": ["Tasks"], "summary": "Retry or cancel a task run", "security": [{"bearerAuth": []}], "parameters": [{"name": "task_id", "in": "path", "required": true, "schema": {"type": "string"}}, {"name": "action", "in": "path", "required": true, "schema": {"type": "string", "enum": ["cancel", "retry"]}}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/schedules": {"get": {"tags": ["Operations"], "summary": "List schedules", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}, "post": {"tags": ["Operations"], "summary": "Create or manage schedules", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/schedules/preview": {"post": {"tags": ["Operations"], "summary": "Preview schedule occurrences", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Occurrences"}}}},
    "/api/v1/schedules/{schedule_id}": {"get": {"tags": ["Operations"], "summary": "Get a schedule", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Schedule"}}}, "post": {"tags": ["Operations"], "summary": "Update a schedule", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Updated schedule"}}}},
    "/api/v1/resources": {"get": {"tags": ["Operations"], "summary": "List resources", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Resource page"}}}, "post": {"tags": ["Operations"], "summary": "Manage resources", "security": [{"bearerAuth": []}], "responses": {"405": {"description": "Method not allowed"}}}},
    "/api/v1/resources/{resource_id}": {"get": {"tags": ["Operations"], "summary": "Get a resource", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Resource"}, "404": {"description": "Not found"}}}, "delete": {"tags": ["Operations"], "summary": "Delete a resource", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Deleted"}, "409": {"description": "Resource is in use"}}}},
    "/api/v1/resources/{resource_id}/lease": {"post": {"tags": ["Operations"], "summary": "Acquire a resource lease", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Lease"}, "409": {"description": "Lease conflict"}}}, "delete": {"tags": ["Operations"], "summary": "Release a resource lease", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Released"}}}},
    "/api/v1/users": {"get": {"tags": ["Administration"], "summary": "List users", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "User page"}}}, "post": {"tags": ["Administration"], "summary": "Create a user", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Created"}}}},
    "/api/v1/users/{user_id}": {"get": {"tags": ["Administration"], "summary": "Get user details", "security": [{"bearerAuth": []}], "parameters": [{"name": "user_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"200": {"description": "User details"}, "403": {"description": "Forbidden"}, "404": {"description": "Not found"}}}},
    "/api/v1/roles": {"get": {"tags": ["Administration"], "summary": "List roles", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}, "post": {"tags": ["Administration"], "summary": "Manage roles", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/sso": {"get": {"tags": ["Administration"], "summary": "List SSO configuration", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}, "post": {"tags": ["Administration"], "summary": "Manage SSO configuration", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/logs": {"get": {"tags": ["Operations"], "summary": "List logs", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runs": {"get": {"tags": ["Runs"], "summary": "List runs", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runs/{run_id}": {"get": {"tags": ["Runs"], "summary": "Get a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Run"}}}},
    "/api/v1/runs/{run_id}/cancel": {"post": {"tags": ["Runs"], "summary": "Cancel a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Cancelled"}}}},
    "/api/v1/runs/{run_id}/retry": {"post": {"tags": ["Runs"], "summary": "Retry a run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Retried"}}}},
    "/api/v1/runs/{run_id}/reconcile": {"post": {"tags": ["Runs"], "summary": "Reconcile an unknown run", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Reconciled"}}}},
    "/api/v1/runs/{run_id}/events": {"get": {"tags": ["Runs"], "summary": "List run events", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Events"}}}},
    "/api/v1/runs/{run_id}/logs": {"get": {"tags": ["Runs"], "summary": "Stream run logs", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Log chunks"}}}},
    "/api/v1/runs/{run_id}/logs/download": {"get": {"tags": ["Runs"], "summary": "Download run logs", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Log download"}}}},
    "/api/v1/events": {"get": {"tags": ["Runs"], "summary": "List events", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runners": {"get": {"tags": ["Runners"], "summary": "List runners", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner page"}}}},
    "/api/v1/runners/{runner_id}": {"get": {"tags": ["Runners"], "summary": "Get a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner"}}}, "delete": {"tags": ["Runners"], "summary": "Permanently delete a runner", "security": [{"bearerAuth": []}], "responses": {"204": {"description": "Deleted"}, "404": {"description": "Not found"}, "409": {"description": "Runner is referenced by execution history"}}}},
    "/api/v1/runners/{runner_id}/drain": {"post": {"tags": ["Runners"], "summary": "Drain a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner updated"}}}},
    "/api/v1/runners/{runner_id}/revoke": {"post": {"tags": ["Runners"], "summary": "Revoke a runner", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Runner updated"}}}},
    "/api/v1/runners/enrollments": {"post": {"tags": ["Runners"], "summary": "Create a one-use runner enrollment", "security": [{"bearerAuth": []}], "responses": {"201": {"description": "Enrollment artifact"}}}},
    "/api/v1/audit": {"get": {"tags": ["Administration"], "summary": "List audit events", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runs/execute": {"post": {"tags": ["Runs"], "summary": "Execute a task", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runs/retry": {"post": {"tags": ["Runs"], "summary": "Retry a run", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/runs/cancel": {"post": {"tags": ["Runs"], "summary": "Cancel a run", "security": [{"bearerAuth": []}], "responses": {"501": {"description": "Not implemented"}}}},
    "/api/v1/admin/auth/settings": {"post": {"tags": ["Administration"], "summary": "Update password authentication settings", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"200": {"description": "Settings updated"}}}},
    "/api/v1/admin/auth/sessions/revoke": {"post": {"tags": ["Administration"], "summary": "Revoke a session", "security": [{"bearerAuth": []}], "parameters": [{"name": "session_id", "in": "query", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "Session revoked"}}}},
    "/api/v1/admin/auth/providers": {"get": {"tags": ["Administration"], "summary": "List OIDC providers", "security": [{"bearerAuth": []}], "responses": {"200": {"description": "Providers"}}}, "post": {"tags": ["Administration"], "summary": "Add or update an OIDC provider", "security": [{"bearerAuth": []}], "requestBody": {"content": {"application/json": {"schema": {"type": "object"}}}}, "responses": {"201": {"description": "Provider updated"}}}},
    "/api/v1/admin/auth/users/{user_id}/disable": {"post": {"tags": ["Administration"], "summary": "Disable a user", "security": [{"bearerAuth": []}], "parameters": [{"name": "user_id", "in": "path", "required": true, "schema": {"type": "string"}}], "responses": {"204": {"description": "User disabled"}}}},
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
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css">
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
    <div id="swagger-ui"></div>
  </main>
  <script src="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js"></script>
  <script>
    let ui;
    window.onload = function () {
      ui = SwaggerUIBundle({url: '/openapi.json', dom_id: '#swagger-ui', presets: [SwaggerUIBundle.presets.apis], layout: 'BaseLayout'});
    };
    document.getElementById('login-form').addEventListener('submit', async function (event) {
      event.preventDefault();
      const status = document.getElementById('login-status');
      status.textContent = 'Signing in…';
      try {
        const response = await fetch('/docs/login', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email: document.getElementById('email').value, password: document.getElementById('password').value})});
        const body = await response.json();
        if (!response.ok) throw new Error(body.error || 'Login failed');
        const token = body.access_token || body.AccessToken;
        if (!token) throw new Error('Login response did not contain an access token');
        ui.preauthorizeApiKey('bearerAuth', token);
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
	writeJSON(w, http.StatusOK, tokens)
}
