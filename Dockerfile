# Stage 1: Build the github-mcp-server binary
FROM golang:1.24.3-alpine AS build
ARG VERSION="dev"

# Set the working directory
WORKDIR /build

# Install git
RUN --mount=type=cache,target=/var/cache/apk \
    apk add git

# Build the server
# go build automatically download required module dependencies to /go/pkg/mod
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION} -X main.commit=$(git rev-parse HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /bin/github-mcp-server cmd/github-mcp-server/main.go

# Stage 2: Run the app
FROM gcr.io/distroless/base-debian12 as mcp-server

# Set the working directory
WORKDIR /server

# Copy the binary from the build stage
COPY --from=build /bin/github-mcp-server .

# Set the entrypoint to the server binary
ENTRYPOINT ["/server/github-mcp-server"]

# Default arguments for ENTRYPOINT
CMD ["stdio"]

# Stage 3: Create the final image with supergateway
FROM ghcr.io/supercorp-ai/supergateway:latest

# Copy the github-mcp-server binary from the first stage
COPY --from=mcp-server /server/github-mcp-server ./github-mcp-server

# Make sure it's executable
RUN chmod +x ./github-mcp-server

EXPOSE 8000

# Run github-mcp-server with supergateway
CMD ["supergateway", "--stdio", "./github-mcp-server stdio"]
