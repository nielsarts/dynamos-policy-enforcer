# DYNAMOS eFLINT Policy Enforcer

A policy enforcement service for the DYNAMOS system that integrates with eFLINT for norm-based policy validation.

## Overview

The Policy Enforcer is a Go-based microservice designed to validate data sharing requests in the DYNAMOS (Dynamically Adaptive Microservice-based OS) platform. It uses [eFLINT](https://gitlab.com/eflint), a domain-specific language for executable norm specifications, to evaluate policies and ensure compliance with consortium agreements.

### Key Features

- **eFLINT Integration**: Manages eFLINT server instances for norm-based reasoning
- **REST API**: HTTP endpoints for instance management and policy commands
- **State Management**: Export/import eFLINT execution states (POC)
- **RabbitMQ Support**: Message queue integration for DYNAMOS orchestration (in development)
- **Docker Support**: Containerized deployment with eFLINT server bundled

## Architecture

```
Data Analyst (HTTP)
    ↓
Gateway (REST API)
    ↓ (RabbitMQ)
Orchestrator ←→ etcd (Knowledge Base)
    ↓ (RabbitMQ: RequestApproval)
Policy Enforcer  ◄── YOU ARE HERE
    ↓ (TCP/JSON)
eFLINT Server (Haskell)
```

## Prerequisites

- **Go**: 1.24 or later
- **eFLINT Server**: The `eflint-server` executable must be available
- **Docker** (optional): For containerized deployment
- **RabbitMQ** (optional): For DYNAMOS integration

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/nielsarts/dynamos-eflint-policy-enforcer.git
cd dynamos-eflint-policy-enforcer

# Download dependencies
go mod download

# Build the binary
go build -o policy-enforcer ./cmd/policy-enforcer
```

### Using Docker

```bash
# Build the Docker image (requires eflint:latest image)
docker build -t dynamos-policy-enforcer .

# Run with Docker Compose
docker-compose up -d
```

> **Note**: The Docker build expects an `eflint:latest` image containing the `eflint-server` binary at `/usr/bin/eflint-server`.

## Configuration

Configuration is managed via YAML file. Default location: `./configs/config.yaml`

```yaml
# RabbitMQ settings
rabbitmq:
  host: localhost
  port: 30020
  username: guest
  password: guest
  queue: policyEnforcer-in
  exchange: topic_exchange
  routing_key: policy.enforcer
  prefetch_count: 10
  reconnect_delay: 5s

# eFLINT server settings
eflint:
  host: localhost
  port: 8123
  server_path: eflint-server  # Path to the eflint-server executable
  model_path: "/eflint/dynamos-agreement.eflint"
  timeout: 30s
  reconnect_delay: 5s
  max_retries: 3

# Logging settings
logging:
  level: info      # debug, info, warn, error
  format: json     # json or console
  output: stdout
  development: false
```

### Environment Variables

| Variable    | Description                | Default |
|-------------|----------------------------|---------|
| `HTTP_PORT` | HTTP server port           | `8080`  |

## Usage

### Running the Service

```bash
# Using default configuration
./policy-enforcer

# With custom configuration file
./policy-enforcer -config /path/to/config.yaml
```

### API Endpoints

The service exposes a REST API for managing eFLINT instances and sending commands.

#### Health Check

```bash
GET /health
```

#### Instance Management

| Method | Endpoint         | Description                          |
|--------|------------------|--------------------------------------|
| GET    | `/eflint/status` | Get eFLINT instance status           |
| POST   | `/eflint/start`  | Start eFLINT instance with model     |
| POST   | `/eflint/stop`   | Stop running eFLINT instance         |

#### Example: Start eFLINT Instance

```bash
curl -X POST http://localhost:8080/eflint/start \
  -H "Content-Type: application/json" \
  -d '{"model_location": "/eflint/dynamos-agreement.eflint"}'
```

#### State Management (POC)

| Method | Endpoint                | Description              |
|--------|-------------------------|--------------------------|
| GET    | `/eflint/state`         | Get current state        |
| POST   | `/eflint/state/export`  | Export state to file     |
| POST   | `/eflint/state/import`  | Import state from file   |

For complete API documentation, see [docs/openapi.yaml](docs/openapi.yaml).

## Project Structure

```
├── cmd/
│   └── policy-enforcer/
│       └── main.go              # Application entry point
├── configs/
│   └── config.yaml              # Default configuration
├── docs/
│   ├── eflint-package.md        # eFLINT documentation
│   └── openapi.yaml             # OpenAPI specification
├── eflint/
│   └── dynamos-agreement.eflint # Default eFLINT policy model
├── internal/
│   ├── config/                  # Configuration loading
│   ├── eflint/                  # eFLINT server management
│   ├── handler/                 # Request handlers
│   └── rabbitmq/                # RabbitMQ consumer
├── pkg/
│   └── proto/                   # Protocol buffer definitions
├── docker-compose.yml
├── Dockerfile
└── go.mod
```

## eFLINT Policy Model

The default policy model (`eflint/dynamos-agreement.eflint`) defines:

- **Organizations**: Data providers in the consortium
- **Requesters**: Users who can submit data requests
- **Agreements**: Registration, authorization, and access control rules
- **Acts**: Administrative operations (register, authorize, revoke)

Example facts and acts:
```eflint
Fact organization        Identified by String
Fact registered-with     Identified by org * req
Fact allowed-data-set    Identified by org * req * dataset

Act register-requester
  Actor      org
  Recipient  req
  Creates    registered-with(org, req)
  Holds when organization(org) && requester(req).
```

## Development

### Building

```bash
go build -o policy-enforcer ./cmd/policy-enforcer
```

### Running Tests

```bash
go test -v ./...
```

### Docker Build

```bash
docker build -t dynamos-policy-enforcer .
```

For detailed development information, see [policy_enforcer_dev_guide.md](policy_enforcer_dev_guide.md).

## Related Projects

- [DYNAMOS](https://github.com/Jorrit05/DYNAMOS) - Main DYNAMOS platform
- [eFLINT](https://gitlab.com/eflint) - Executable norm specification language

## License

This project is licensed under the terms specified in the [LICENSE](LICENSE) file.

## Author

Niels Arts
