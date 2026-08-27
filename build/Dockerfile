# syntax=docker/dockerfile:1

FROM node:22.22.2-bookworm-slim@sha256:9f6d5975c7dca860947d3915877f85607946403fc55349f39b4bc3688448bb6e AS frontend-build
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY assets/ /src/assets/
COPY frontend/ ./
COPY VERSION /src/VERSION
RUN npm run build

FROM ubuntu:24.04@sha256:33ceb71981b602c1a7443a53469e4dba065f7503eab3078a2d7a57a2ab987517 AS backend-build
ARG GO_VERSION=1.25.13
ARG GO_SHA256=39042a078ea9ceebe3ecda4a7188f0f5b96e14a071d27923ba7f40b456e85ae3
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        curl \
        git \
        libgtk-3-dev \
        libgtk-4-dev \
        libx11-xcb-dev \
        libxkbcommon-x11-dev \
        libwebkit2gtk-4.1-dev \
        libwebkitgtk-6.0-dev \
        libsoup-3.0-dev \
        pkg-config \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL "https://dl.google.com/go/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz \
    && echo "${GO_SHA256}  /tmp/go.tar.gz" | sha256sum -c - \
    && tar -C /usr/local -xzf /tmp/go.tar.gz \
    && rm /tmp/go.tar.gz
ENV PATH="/usr/local/go/bin:/go/bin:${PATH}" \
    GOPATH=/go
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY VERSION /src/VERSION
RUN version="$(tr -d '[:space:]' < /src/VERSION)" && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X github.com/VBenevides/Glyphflow/backend.Version=$version" -o /out/glyphflow-controlplane ./cmd/controlplane
RUN bash ./build_runner_binaries.sh

FROM nginx:1.29-alpine@sha256:5616878291a2eed594aee8db4dade5878cf7edcb475e59193904b198d9b830de
WORKDIR /app
ENV CORS_ORIGIN="*" \
    GLYPHFLOW_DISABLE_NGINX="false"
COPY --from=frontend-build /src/frontend/dist /usr/share/nginx/html
COPY --from=backend-build /out/glyphflow-controlplane /usr/local/bin/glyphflow-controlplane
COPY --from=backend-build /src/backend/migrations ./migrations
COPY --from=backend-build /src/backend/runner-binaries ./runner-binaries
COPY build/nginx.conf /etc/nginx/nginx.conf
COPY LICENSE THIRD-PARTY-NOTICES /usr/share/doc/glyphflow/
CMD ["sh", "-c", "if [ \"${GLYPHFLOW_DISABLE_NGINX:-false}\" = \"true\" ]; then exec /usr/local/bin/glyphflow-controlplane; else exec nginx -g 'daemon off;'; fi"]
