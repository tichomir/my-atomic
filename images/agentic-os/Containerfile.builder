# Agentic OS - Go daemon builder stage
# Produces /out/atomicagentd and /out/atomic-agent-ctl
# Build: podman build -f images/agentic-os/Containerfile.builder -t agentic-os-builder:latest .
FROM golang:1.24-bookworm

WORKDIR /build

# Cache dependencies first
COPY daemon/go.mod daemon/go.sum ./
RUN go mod download

# Build daemon and CLI
ARG BUILD_VERSION=dev
COPY daemon/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${BUILD_VERSION}" \
    -o /out/atomicagentd ./cmd/atomicagentd && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=${BUILD_VERSION}" \
    -o /out/atomic-agent-ctl ./cmd/atomic-agent-ctl

# Verify binaries built correctly
RUN /out/atomicagentd --version && /out/atomic-agent-ctl --version
