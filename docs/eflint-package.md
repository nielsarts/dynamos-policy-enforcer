# DYNAMOS Policy Enforcer Documentation

The DYNAMOS Policy Enforcer provides modular policy enforcement for data sharing agreements. It uses a reasoner-agnostic architecture that can work with different policy reasoning engines (eFLINT, Symboleo, JSON-based, etc.).

## Package Structure

```
internal/
├── reasoner/           # Reasoner abstraction layer
│   ├── reasoner.go         # Reasoner interface definition
│   └── eflint_reasoner.go  # eFLINT implementation
├── policyenforcer/     # High-level policy enforcement
│   ├── types.go            # Request/response types
│   ├── enforcer.go         # Core enforcement service
│   └── http_handler.go     # HTTP API handlers
├── eflint/             # eFLINT server management (low-level)
│   ├── errors.go           # Sentinel errors
│   ├── instance.go         # Instance data structure
│   ├── instance_api.go     # HTTP API for instance management
│   ├── manager.go          # Process lifecycle management
│   ├── state_api.go        # HTTP API for state management (POC)
│   └── state_manager.go    # State persistence (POC)
├── handler/            # RabbitMQ message handling
│   └── handler.go
└── config/             # Configuration
    └── config.go
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           HTTP / gRPC Layer                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│  Policy Enforcer API              │  eFLINT API                             │
│  /policy-enforcer/*               │  /eflint/*                              │
│  (Reasoner-agnostic)              │  (eFLINT-specific, low-level)           │
└────────────────┬──────────────────┴───────────────────┬─────────────────────┘
                 │                                       │
                 ▼                                       │
┌─────────────────────────────────────┐                  │
│         Reasoner Interface          │                  │
│  ┌─────────────────────────────┐    │                  │
│  │   EflintReasoner (current)  │    │                  │
│  └─────────────────────────────┘    │                  │
│  ┌─────────────────────────────┐    │                  │
│  │  SymboleoReasoner (future)  │    │                  │
│  └─────────────────────────────┘    │                  │
│  ┌─────────────────────────────┐    │                  │
│  │    JSONReasoner (future)    │    │                  │
│  └─────────────────────────────┘    │◄─────────────────┘
└────────────────┬────────────────────┘
                 │
                 ▼
┌─────────────────────────────────────┐
│           eFLINT Manager            │
│   - Process lifecycle               │
│   - TCP communication               │
└────────────────┬────────────────────┘
                 │
                 ▼ TCP
┌─────────────────────────────────────┐
│          eFLINT Server              │
│       (external process)            │
└─────────────────────────────────────┘
```

## Components

### Reasoner Interface (`internal/reasoner/`)

The `Reasoner` interface provides an abstraction layer for policy reasoning engines. This enables modularity - different reasoning backends can be swapped without changing the policy enforcement logic.

```go
type Reasoner interface {
    // Query allowed clauses for a requester at an organization
    GetAllowedRequestTypes(ctx context.Context, organization, requester string) ([]string, error)
    GetAllowedDataSets(ctx context.Context, organization, requester string) ([]string, error)
    GetAllowedArchetypes(ctx context.Context, organization, requester string) ([]string, error)
    GetAllowedComputeProviders(ctx context.Context, organization, requester string) ([]string, error)
    
    // Validate if a specific request is allowed
    IsRequestAllowed(ctx context.Context, params RequestParams) (*RequestValidationResult, error)
    
    // Status
    IsRunning() bool
    Name() string
}
```

Optional interfaces for extended functionality:

```go
// For organization-level availability queries
type AvailabilityProvider interface {
    GetAvailableArchetypes(ctx context.Context, organization string) ([]string, error)
    GetAvailableComputeProviders(ctx context.Context, organization string) ([]string, error)
}

// For state management (checkpointing, export/import)
type StateManager interface {
    ExportState(ctx context.Context) ([]byte, error)
    ImportState(ctx context.Context, state []byte) error
}
```

### eFLINT Reasoner (`internal/reasoner/eflint_reasoner.go`)

Implements the `Reasoner` interface using an eFLINT server:

```go
// Create the eFLINT reasoner
eflintReasoner := reasoner.NewEflintReasoner(manager, logger)

// Use it through the interface
archetypes, err := eflintReasoner.GetAllowedArchetypes(ctx, "VU", "user@example.com")

// Validate a request
result, err := eflintReasoner.IsRequestAllowed(ctx, reasoner.RequestParams{
    Organization:    "VU",
    Requester:       "user@example.com",
    RequestType:     "sqlDataRequest",
    DataSet:         "wageGap",
    Archetype:       "dataThroughTtp",
    ComputeProvider: "SURF",
})
```

### Policy Enforcer (`internal/policyenforcer/`)

High-level policy enforcement service that uses the Reasoner interface:

```go
// Create the enforcer with any Reasoner implementation
enforcer := policyenforcer.NewEnforcer(eflintReasoner, logger)

// Get all allowed clauses at once
clauses, err := enforcer.GetAllAllowedClauses(ctx, "VU", "user@example.com")
// clauses.RequestTypes, clauses.DataSets, clauses.Archetypes, clauses.ComputeProviders

// Validate a request
result, err := enforcer.ValidateRequest(ctx, &policyenforcer.ValidateRequestParams{
    Organization:    "VU",
    Requester:       "user@example.com",
    RequestType:     "sqlDataRequest",
    DataSet:         "wageGap",
    Archetype:       "dataThroughTtp",
    ComputeProvider: "SURF",
})
if result.Allowed {
    // Request is permitted
}
```

### eFLINT Manager (`internal/eflint/manager.go`)

Manages the lifecycle of the eFLINT server instance:

```go
config := eflint.DefaultManagerConfig()
manager := eflint.NewManager(config, logger)

// Start with a model
err := manager.Start("/path/to/model.eflint")

// Send a raw command (low-level)
response, err := manager.SendCommand(`{"command": "facts"}`)

// Stop the instance
manager.Stop()
```

### State Manager (`internal/eflint/state_manager.go`) - POC

State persistence for checkpointing:

```go
stateManager := eflint.NewStateManager(manager, "eflint-states", logger)

// Create checkpoint before making changes
state, err := stateManager.CreateCheckpoint("before-test")

// ... make changes ...

// Restore to checkpoint if needed
err := stateManager.RestoreCheckpoint("before-test")
```

> ⚠️ **Note**: Due to limitations in the eFLINT server's `load-export` functionality, full state restoration may not work in all cases.

## API Endpoints

### Policy Enforcer API (`/policy-enforcer/*`)

High-level, reasoner-agnostic endpoints for policy enforcement:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/policy-enforcer/info` | Get active reasoner info |
| GET | `/policy-enforcer/allowed-request-types` | Get allowed request types |
| GET | `/policy-enforcer/allowed-data-sets` | Get allowed datasets |
| GET | `/policy-enforcer/allowed-archetypes` | Get allowed archetypes |
| GET | `/policy-enforcer/allowed-compute-providers` | Get allowed compute providers |
| GET | `/policy-enforcer/allowed-clauses` | Get all allowed clauses at once |
| POST | `/policy-enforcer/validate` | Validate if a request is allowed |
| GET | `/policy-enforcer/available-archetypes` | Get available archetypes (org-level) |
| GET | `/policy-enforcer/available-compute-providers` | Get available providers (org-level) |

**Query parameters for allowed-* endpoints:**
- `organization` (required): Organization/steward identifier (e.g., "VU")
- `requester` (required): Requester identifier (e.g., "user@example.com")

**Example: Validate a request**
```bash
curl -X POST http://localhost:8080/policy-enforcer/validate \
  -H "Content-Type: application/json" \
  -d '{
    "organization": "VU",
    "requester": "jorrit.stutterheim@cloudnation.nl",
    "request_type": "sqlDataRequest",
    "data_set": "wageGap",
    "archetype": "dataThroughTtp",
    "compute_provider": "SURF"
  }'
```

### eFLINT API (`/eflint/*`)

Low-level, eFLINT-specific endpoints for server management:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/eflint/status` | Get instance status |
| POST | `/eflint/start` | Start instance with model |
| POST | `/eflint/stop` | Stop running instance |
| POST | `/eflint/command` | Send raw command to eFLINT |

### State Management API (`/eflint/state/*`) - POC

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/eflint/state` | Get current execution graph |
| POST | `/eflint/state/export` | Export state for persistence |
| POST | `/eflint/state/import` | Import saved state |
| POST | `/eflint/state/checkpoint` | Create named checkpoint |
| POST | `/eflint/state/checkpoint/restore` | Restore checkpoint |
| GET | `/eflint/state/checkpoints` | List checkpoints |
| DELETE | `/eflint/state/checkpoint/:name` | Delete checkpoint |

## Registering Routes

```go
e := echo.New()

// eFLINT Manager
manager := eflint.NewManager(eflint.DefaultManagerConfig(), logger)
stateManager := eflint.NewStateManager(manager, "eflint-states", logger)

// eFLINT API (low-level)
eflintGroup := e.Group("/eflint")
eflint.NewInstanceAPIHandler(manager, logger).RegisterRoutes(eflintGroup)
eflint.NewStateAPIHandler(stateManager, logger).RegisterRoutes(eflintGroup)

// Policy Enforcer API (high-level, reasoner-agnostic)
eflintReasoner := reasoner.NewEflintReasoner(manager, logger)
enforcer := policyenforcer.NewEnforcer(eflintReasoner, logger)
policyEnforcerGroup := e.Group("/policy-enforcer")
policyenforcer.NewHTTPHandler(enforcer, logger).RegisterRoutes(policyEnforcerGroup)
```

## Adding a New Reasoner

To add support for a new reasoning engine (e.g., Symboleo):

1. Create `internal/reasoner/symboleo_reasoner.go`:

```go
package reasoner

type SymboleoReasoner struct {
    // ... implementation details
}

func NewSymboleoReasoner(config SymboleoConfig, logger *zap.Logger) *SymboleoReasoner {
    // ...
}

func (r *SymboleoReasoner) GetAllowedArchetypes(ctx context.Context, org, req string) ([]string, error) {
    // Implement using Symboleo-specific logic
}

func (r *SymboleoReasoner) IsRequestAllowed(ctx context.Context, params RequestParams) (*RequestValidationResult, error) {
    // Implement using Symboleo-specific logic
}

// ... implement other interface methods

var _ Reasoner = (*SymboleoReasoner)(nil) // Compile-time check
```

2. In `main.go`, select the reasoner based on configuration:

```go
var r reasoner.Reasoner
switch cfg.Reasoner.Type {
case "eflint":
    r = reasoner.NewEflintReasoner(manager, logger)
case "symboleo":
    r = reasoner.NewSymboleoReasoner(symboleoConfig, logger)
case "json":
    r = reasoner.NewJSONReasoner(jsonPath, logger)
}
enforcer := policyenforcer.NewEnforcer(r, logger)
```

## Error Handling

Sentinel errors in `internal/eflint/errors.go`:

| Error | Description |
|-------|-------------|
| `ErrInstanceNotFound` | Instance does not exist |
| `ErrInstanceNotRunning` | Instance exists but process has exited |
| `ErrInstanceAlreadyExists` | Instance already in use |
| `ErrProcessStartFailed` | Failed to start eFLINT server |
| `ErrConnectionFailed` | TCP connection failed |
| `ErrCommandFailed` | Failed to send command |
| `ErrStateExportFailed` | State export failed |
| `ErrStateImportFailed` | State import failed |

```go
if errors.Is(err, eflint.ErrInstanceNotFound) {
    // Handle missing instance
}
```

## Configuration

```go
type ManagerConfig struct {
    EflintServerPath  string        // Path to eflint-server executable
    MinPort           int           // Minimum port for random selection
    MaxPort           int           // Maximum port for random selection
    StartupDelay      time.Duration // Wait time after starting process
    ConnectionTimeout time.Duration // TCP connection/command timeout
}
```

Default values:
- `EflintServerPath`: `"eflint-server"`
- `MinPort`: `1025`
- `MaxPort`: `65535`
- `StartupDelay`: `3 seconds`
- `ConnectionTimeout`: `60 seconds`

## Design Principles

- **Modularity**: Reasoner interface allows swapping policy engines
- **SOLID**: Single responsibility per component
- **DRY**: Shared types and error definitions
- **KISS**: Simple, focused interfaces
- **Protocol-agnostic**: HTTP now, gRPC support ready for future

## Thread Safety

All components are designed for concurrent access:
- `Instance`: Uses `sync.RWMutex` for field access
- `Manager`: Uses `sync.RWMutex` for instance operations
- `StateManager`: Uses `sync.RWMutex` for state operations
- `Enforcer`: Stateless, thread-safe by design

## Dependencies

- `github.com/labstack/echo/v4` - HTTP framework
- `go.uber.org/zap` - Structured logging
- `github.com/spf13/viper` - Configuration
- Standard library: `os/exec`, `net`, `encoding/json`, `sync`, `context`
