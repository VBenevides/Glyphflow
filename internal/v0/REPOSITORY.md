# Repository layout

```text
backend/    Go module with shared packages, control plane, and worker commands
frontend/   TypeScript and React application
internal/   Architecture, migration, roadmap, and operator documentation
local/      Local-only development assets and imported scheduler data
assets/     Project artwork and other static assets
```

Backend code stays in the Go module under `backend/`. Frontend code stays in
the frontend application. Shared protocol types belong in backend packages
that both commands can import; neither executable imports frontend code.
