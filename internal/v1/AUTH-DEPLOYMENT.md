# Authentication deployment modes

Password-only mode enables password login and, optionally, registration. SSO-only mode disables password login and registration but keeps one or more enabled OIDC providers. Mixed mode enables both. Every first user must follow the bootstrap administrator path; production startup must have a recoverable administrator.

OIDC client secrets are configured as versioned `secret://` references. Callback URLs must be HTTPS and match the provider allow-list. Set `HttpOnly`, `Secure` (outside local development), narrow-path, explicit SameSite cookies. Configure the reverse proxy to preserve the original HTTPS origin, and set `WEB_ORIGIN` so unsafe requests pass origin and CSRF checks.

Use TLS for the browser, database, NATS, and OIDC provider. Rotate access-token and provider credentials through overlapping validity windows. Recovery is performed by an administrator or deployment operator; never copy password, refresh-token, PKCE, or resolved secret values into logs.
