# syntax=docker/dockerfile:1

# Build the application from source
FROM golang:1.24 AS build-stage

WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /policy-enforcer ./cmd/policy-enforcer

# Run the tests in the container
FROM build-stage AS run-test-stage
RUN go test -v ./...

# Deploy the application binary into a lean image
# Using debian:bookworm-slim because eflint-server (Haskell binary) needs glibc and other libs
FROM debian:bookworm-slim AS build-release-stage

WORKDIR /

# Install required libraries for Haskell binary
RUN apt-get update && apt-get install -y --no-install-recommends \
    libgmp10 \
    libc6 \
    libffi8 \
    libnuma1 \
    && rm -rf /var/lib/apt/lists/*

# Copy eflint-server executable from the eflint image
COPY --from=eflint:latest /usr/bin/eflint-server /usr/bin/eflint-server

# Copy the binary from build stage
COPY --from=build-stage /policy-enforcer /policy-enforcer

# Copy configuration files
COPY --from=build-stage /app/configs /configs

# Expose HTTP port
EXPOSE 8080

ENTRYPOINT ["/policy-enforcer"]
