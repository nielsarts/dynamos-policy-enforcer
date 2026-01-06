# eFLINT Package Documentation

The `internal/eflint` package provides functionality for managing and communicating with eFLINT server instances for policy enforcement in the DYNAMOS system.

## Package Structure

```
internal/eflint/
├── errors.go        # Sentinel errors and error types
├── instance.go      # Instance data structure
├── instance_api.go  # HTTP API handler for instance management
├── manager.go       # Manager for process lifecycle
├── state_api.go     # HTTP API handler for state management (POC)
└── state_manager.go # State persistence and checkpoints (POC)
```

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        HTTP Layer                                │
├─────────────────────────────────────────────────────────────────┤
│  InstanceAPIHandler            │  StateAPIHandler               │
│  /eflint/*                     │  /eflint/state/*               │
└─────────────┬──────────────────┴────────────────┬───────────────┘
              │                                    │
              ▼                                    ▼
┌─────────────────────────────┐    ┌─────────────────────────────┐
│         Manager             │◄───│      StateManager           │
│   - Process lifecycle       │    │   - Get state               │
│   - TCP communication       │    │   - Export/Import           │
└─────────────┬───────────────┘    │   - Checkpoints             │
              │                    └─────────────────────────────┘
              ▼
┌─────────────────────────────┐
│         Instance            │
│   - Process handle          │
│   - Port & metadata         │
└─────────────┬───────────────┘
              │
              ▼ TCP
┌─────────────────────────────┐
│      eFLINT Server          │
│   (external process)        │
└─────────────────────────────┘
```

## Components

### Instance (`instance.go`)

Represents a running eFLINT server process. Thread-safe data structure that encapsulates:

- **Port**: TCP port the server listens on
- **Process**: Handle to the running process
- **ModelLocation**: Path to the loaded eFLINT model

```go
instance := eflint.NewInstance(port, process, modelLocation)
if instance.IsAlive() {
    // Instance is running
}
instance.Kill() // Terminate the process
```

### Manager (`manager.go`)

Manages the lifecycle of the eFLINT server instance. Provides:

- **Start/Stop/Restart**: Process lifecycle management
- **SendCommand**: Send JSON commands via TCP
- **GetState**: Retrieve current eFLINT state
- **UpdateModel**: Hot-reload with a new model

```go
config := eflint.DefaultManagerConfig()
manager := eflint.NewManager(config, logger)

// Start with a model
err := manager.Start("/path/to/model.eflint")

// Send a command
response, err := manager.SendCommand(`{"command": "phrase", "text": "tick()"}`)

// Check status
status := manager.Status()
fmt.Printf("Running: %v, Port: %d\n", status.Running, status.Port)

// Stop the instance
manager.Stop()
```

### StateManager (`state_manager.go`)

**POC (Proof of Concept)** for state persistence. Provides:

- **Export/Import**: Save and restore eFLINT execution graphs
- **Checkpoints**: Named snapshots for rollback scenarios

> ⚠️ **Note**: Due to limitations in the eFLINT server's `load-export` functionality, full state restoration may not work in all cases.

```go
stateManager := eflint.NewStateManager(manager, "/tmp/states", logger)

// Export current state
state, err := stateManager.ExportState()

// Create a checkpoint
state, err := stateManager.CreateCheckpoint("before-test")

// Restore checkpoint
err := stateManager.RestoreCheckpoint("before-test")
```

### API Handlers (`instance_api.go`, `state_api.go`)

HTTP handlers using Echo framework. Register with Echo groups:

```go
eflintGroup := e.Group("/eflint")
instanceAPIHandler.RegisterRoutes(eflintGroup)

stateGroup := e.Group("/eflint/state")
stateAPIHandler.RegisterRoutes(stateGroup)
```

## Error Handling

The package defines sentinel errors in `errors.go`:

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
| `ErrInvalidResponse` | Invalid eFLINT response format |

Use `errors.Is()` for error checking:

```go
if errors.Is(err, eflint.ErrInstanceNotFound) {
    // Handle missing instance
}
```

The `InstanceError` type wraps errors with instance context:

```go
if ie, ok := err.(*eflint.InstanceError); ok {
    fmt.Printf("Error for instance: %v\n", ie.Err)
}
```

## API Endpoints

### Instance Management (`/eflint/*`)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/eflint/status` | Get instance status |
| POST | `/eflint/start` | Start instance with model (use `force=true` to restart) |
| POST | `/eflint/stop` | Stop running instance |
| POST | `/eflint/command` | Send command to eFLINT |

### State Management (`/eflint/state/*`) - POC

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/eflint/state` | Get current execution graph state |
| POST | `/eflint/state/export` | Export state with metadata for persistence |
| POST | `/eflint/state/import` | Import saved state |
| POST | `/eflint/state/checkpoint` | Create named checkpoint |
| POST | `/eflint/state/checkpoint/restore` | Restore checkpoint |
| GET | `/eflint/state/checkpoints` | List checkpoints |
| DELETE | `/eflint/state/checkpoint/:name` | Delete checkpoint |

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

This package follows software engineering best practices:

- **SOLID**: Single responsibility per component
- **DRY**: Shared error definitions and response types
- **KISS**: Simple, focused interfaces
- **YAGNI**: Removed unused multi-instance manager

## Thread Safety

All components are designed for concurrent access:
- `Instance`: Uses `sync.RWMutex` for field access
- `Manager`: Uses `sync.RWMutex` for instance operations
- `StateManager`: Uses `sync.RWMutex` for state operations

## Dependencies

- `github.com/labstack/echo/v4` - HTTP framework
- `go.uber.org/zap` - Structured logging
- Standard library: `os/exec`, `net`, `encoding/json`, `sync`
