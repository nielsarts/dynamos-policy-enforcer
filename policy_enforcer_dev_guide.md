# Policy Enforcer Development Guide

## Project Overview

**Project**: DYNAMOS Policy Enforcer with eFLINT Integration
**Language**: Go
**Purpose**: Replace stubbed policy enforcer in DYNAMOS to enable real policy validation using eFLINT norm reasoning

## System Context

### What is DYNAMOS?
DYNAMOS is a **Dynamically Adaptive Microservice-based OS** for secure data exchange in Digital Data Marketplaces (DDM). It enables organizations like universities to share data while maintaining sovereignty and compliance with policies like GDPR.

### Use Case: UNL (Universities of the Netherlands)
A **Data Analyst** submits SQL queries to analyze student data across multiple Dutch universities (UVA, VU Amsterdam, Leiden). The system must:
- Validate the request against consortium policies
- Select appropriate data sharing archetype (ComputeToData or DataThroughTtp)
- Deploy microservices to execute the query securely
- Return anonymized/aggregated results

## Architecture

### Component Relationships

```
Data Analyst (HTTP)
    ↓
Gateway (REST API)
    ↓ (RabbitMQ)
Orchestrator ←→ etcd (Knowledge Base)
    ↓ (RabbitMQ: RequestApproval)
Policy Enforcer (YOUR COMPONENT!)
    ↓ (TCP/JSON)
eFLINT Server (Haskell)
```

### Your Component: Policy Enforcer

**Location in Flow**: Between Orchestrator and eFLINT Server
**Input**: `RequestApproval` messages from RabbitMQ
**Output**: `ValidationResponse` messages to RabbitMQ
**Side Communication**: TCP/JSON to eFLINT Server

## Communication Protocols

### 1. RabbitMQ (Input/Output)
**Protocol**: AMQP
**Message Format**: Protocol Buffers (protobuf)
**Library**: `github.com/rabbitmq/amqp091-go`

**Consume From**:
- Queue: `policy-enforcer-requests`
- Exchange: `dynamos`
- Routing Key: `request.approval.*`

**Publish To**:
- Exchange: `dynamos`
- Routing Key: Dynamic (specified in request's `destination_queue`)

### 2. eFLINT Server (TCP)
**Protocol**: TCP with JSON messages
**Format**: Line-delimited JSON
**Library**: Standard `net` package

**eFLINT Server Details**:
- Host: Configurable (e.g., `localhost:8001`)
- Connection: Persistent with connection pooling
- Message Format: One JSON object per line

## Existing DYNAMOS Proto Definitions

### RequestApproval (Input Message)

**Actual DYNAMOS Format**:

```protobuf
message RequestApproval {
  string type = 1;  // "sqlDataRequest", "genericRequest"
  User user = 2;
  repeated string data_providers = 3;  // ["VU", "UVA", "RUG"]
  string destination_queue = 4;
  map<string, bool> options = 5;
  DataRequest data_request = 6;  // Contains query, algorithm, etc.
}

message User {
  string id = 1;         // "12324"
  string user_name = 2;  // "jorrit.stutterheim@cloudnation.nl"
}

message DataRequest {
  string type = 1;      // "sqlDataRequest"
  string query = 2;     // SQL query string
  string algorithm = 3; // "average", "sum", etc.
  map<string, string> algorithm_columns = 4;
  map<string, bool> options = 5;
  RequestMetadata request_metadata = 6;
}
```

**Example JSON representation**:
```json
{
    "type": "sqlDataRequest",
    "user": {
        "ID": "12324",
        "userName": "jorrit.stutterheim@cloudnation.nl"
    },
    "dataProviders": ["VU", "UVA", "TEST"],
    "data_request": {
        "type": "sqlDataRequest",
        "query": "SELECT * FROM Personen p JOIN Aanstellingen s LIMIT 1000",
        "algorithm": "average"
    }
}
```

### ValidationResponse (Output Message)

```protobuf
message ValidationResponse {
  string type = 1;  // "ValidationResponse"
  string request_type = 2;
  map<string, DataProvider> valid_dataproviders = 3;
  repeated string invalid_dataproviders = 4;
  Auth auth = 5;
  User user = 6;
  bool request_approved = 7;
  UserArchetypes valid_archetypes = 8;
  map<string,bool> options = 9;
}

message DataProvider {
  repeated string archetypes = 1;
  repeated string compute_providers = 2;
}

message UserArchetypes {
  string user_name = 1;
  map<string, UserAllowedArchetypes> archetypes = 2;
}

message UserAllowedArchetypes {
  repeated string archetypes = 1;
}
```

**You need to add** fields for eFLINT integration:
```protobuf
// NEW FIELDS to add
string policy_version = 10;           // Which policy version was evaluated
repeated string violated_norms = 11;  // List of violated norm names
string denial_reason = 12;            // Human-readable explanation
```

## eFLINT Communication

### Actual eFLINT Server Protocol

The eFLINT server uses a JSON-based protocol over TCP. Commands and responses are JSON objects.

**Command Types**:

1. **Execute Action** (most common for validation)
```json
{
    "command": "action",
    "act-type": "submit-request",
    "actor": "jorrit.stutterheim@cloudnation.nl",
    "recipient": "VU",
    "objects": ["sqlDataRequest", "wageGap", "computeToData", "SURF"],
    "force": "false"
}
```

Alternative syntax using value:
```json
{
    "command": "action",
    "value": {
        "tagged-type": "Act",
        "fact-type": "submit-request",
        "value": [...]
    },
    "force": "false"
}
```

2. **Query Enabled** (check if action is possible)
```json
{
    "command": "enabled",
    "value": {
        "tagged-type": "Act",
        "fact-type": "submit-request",
        "value": [
            {"tagged-type": "String", "fact-type": "requester", "value": "jorrit.stutterheim@cloudnation.nl", "textual": "\"jorrit.stutterheim@cloudnation.nl\""},
            {"tagged-type": "String", "fact-type": "organization", "value": "VU", "textual": "\"VU\""},
            {"tagged-type": "String", "fact-type": "request-type", "value": "sqlDataRequest", "textual": "\"sqlDataRequest\""},
            {"tagged-type": "String", "fact-type": "data-set", "value": "wageGap", "textual": "\"wageGap\""},
            {"tagged-type": "String", "fact-type": "archetype", "value": "computeToData", "textual": "\"computeToData\""},
            {"tagged-type": "String", "fact-type": "compute-provider", "value": "SURF", "textual": "\"SURF\""}
        ],
        "textual": "submit-request(\"jorrit.stutterheim@cloudnation.nl\", \"VU\", \"sqlDataRequest\", \"wageGap\", \"computeToData\", \"SURF\")"
    }
}
```

3. **Test Present/Absent**
```json
{
    "command": "test-present",
    "value": {...}
}
```

4. **Create/Terminate Facts**
```json
{
    "command": "create",
    "value": {...}
}
```

5. **Execute Phrase** (for administrative commands)
```json
{
    "command": "phrase",
    "text": "+revoke-archetype(\"VU\", \"jorrit.stutterheim@cloudnation.nl\", \"computeToData\")."
}
```

6. **Get Facts** (retrieve current state)
```json
{
    "command": "facts"
}
```

7. **Get Types**
```json
{
    "command": "types"
}
```

8. **Get Status**
```json
{
    "command": "status"
}
```

9. **Get History/Path**
```json
{
    "command": "history",
    "value": 5  // optional state ID
}
```

10. **Revert to State**
```json
{
    "command": "revert",
    "value": 0,  // state ID (negative for initial state)
    "destructive": false  // optional, default false
}
```

11. **Kill Server**
```json
{
    "command": "kill"
}
```

### eFLINT Response Format

**Status Response** (most common):
```json
{
    "response": "success",
    "old-state": 0,
    "new-state": 1,
    
    "source_contents": [...],
    "target_contents": [...],
    "created_facts": [...],
    "terminated_facts": [...],
    
    "violations": [],
    "output-events": [],
    "errors": [],
    "query-results": [],
    
    "new-duties": [],
    "new-enabled-transitions": [...],
    "new-disabled-transitions": [],
    "all-duties": [],
    "all-enabled-transitions": [...],
    "all-disabled-transitions": [...]
}
```

**Facts Response**:
```json
{
    "values": [...]
}
```

**Path Response**:
```json
{
    "edges": [
        {
            "phrase": "+organization(\"VU\").",
            "source_id": 0,
            "target_id": 1,
            "source_contents": [...],
            "target_contents": [...],
            "created_facts": [...],
            "terminated_facts": [...],
            "violations": [],
            "output-events": [],
            "errors": [],
            "query-results": [],
            "new-duties": [],
            "new-enabled-transitions": [...],
            "new-disabled-transitions": [],
            "all-duties": [],
            "all-enabled-transitions": [...],
            "all-disabled-transitions": [...]
        }
    ]
}
```

**Head/Leaf Nodes Response**:
```json
{
    "state_id": 5,
    "state_contents": [...],
    "duties": [],
    "enabled-transitions": [...],
    "disabled-transitions": [...]
}
```

**Invalid Responses**:
- `{"response": "invalid state"}`
- `{"response": "invalid command"}`
- `{"response": "invalid input"}`

**Killed Response**:
```json
{
    "response": "bye world..."
}
```

### VALUE Format in eFLINT

eFLINT uses a structured format for values:

**Atom (simple value)**:
```json
{
    "tagged-type": "String",
    "fact-type": "requester",
    "value": "jorrit.stutterheim@cloudnation.nl",
    "textual": "\"jorrit.stutterheim@cloudnation.nl\""
}
```

For integers:
```json
{
    "tagged-type": "Integer",
    "fact-type": "some-number",
    "value": 42,
    "textual": "42"
}
```

**Composite (complex value)**:
```json
{
    "tagged-type": "Act",
    "fact-type": "submit-request",
    "value": [
        {"tagged-type": "String", "fact-type": "requester", "value": "user@example.com", "textual": "\"user@example.com\""},
        {"tagged-type": "String", "fact-type": "organization", "value": "VU", "textual": "\"VU\""}
    ],
    "textual": "submit-request(\"user@example.com\", \"VU\", ...)"
}
```

### Violation Format

```json
{
    "violation": "trigger",  // or "duty" or "invariant"
    "value": {
        "tagged-type": "Act",
        "fact-type": "submit-request",
        "value": [...],
        "textual": "submit-request(...)"
    }
}
```

For invariant violations:
```json
{
    "violation": "invariant",
    "invariant": "some-invariant-name"
}
```

## Project Structure

Recommended Go package structure:

```
policy-enforcer/
├── cmd/
│   └── policy-enforcer/
│       └── main.go                 # Entry point
├── internal/
│   ├── rabbitmq/
│   │   ├── consumer.go            # RabbitMQ consumer
│   │   └── publisher.go           # RabbitMQ publisher
│   ├── eflint/
│   │   ├── client.go              # TCP client for eFLINT
│   │   ├── pool.go                # Connection pool
│   │   ├── translator.go          # RequestApproval → eFLINT commands
│   │   └── parser.go              # eFLINT response → ValidationResponse
│   ├── cache/
│   │   └── decision_cache.go      # Cache for policy decisions
│   ├── handler/
│   │   └── request_handler.go     # Main request handler
│   └── config/
│       └── config.go               # Configuration management
├── pkg/
│   └── proto/                      # Generated protobuf (from DYNAMOS repo)
├── configs/
│   └── config.yaml                 # Configuration file
├── go.mod
└── go.sum
```

## Implementation Strategy

### Phase 1: Basic TCP Communication (Start Here!)

**Goal**: Establish basic connection to eFLINT server and send simple commands.

**Steps**:
1. Create `internal/eflint/client.go`
   - `NewClient(addr string) (*Client, error)`
   - `Execute(ctx context.Context, cmd Command) (*Response, error)`
   - `Close() error`

2. Write basic test
   - Connect to eFLINT server
   - Send a simple query: `{"command": "query", "expression": "1 + 1"}`
   - Verify response

### Phase 2: RabbitMQ Integration

**Goal**: Consume RequestApproval messages from RabbitMQ.

**Steps**:
1. Create `internal/rabbitmq/consumer.go`
   - Connect to RabbitMQ
   - Declare queue: `policy-enforcer-requests`
   - Consume messages
   - Deserialize protobuf

2. Create `internal/rabbitmq/publisher.go`
   - Publish ValidationResponse
   - Use routing key from request

### Phase 3: Translation Layer

**Goal**: Convert RequestApproval to eFLINT commands.

**Steps**:
1. Create `internal/eflint/translator.go`
   - `TranslateRequest(req *pb.RequestApproval) []Command`
   - Generate assert commands for facts
   - Generate query command for permission check

### Phase 4: Response Formatting

**Goal**: Convert eFLINT response to ValidationResponse.

**Steps**:
1. Create `internal/eflint/parser.go`
   - `ParseResponse(eflintResp *Response) *pb.ValidationResponse`
   - Determine archetypes based on policy decision
   - Format denial reasons

### Phase 5: Request Handler

**Goal**: Tie everything together.

**Steps**:
1. Create `internal/handler/request_handler.go`
   - Consume from RabbitMQ
   - Translate to eFLINT
   - Execute against eFLINT
   - Parse response
   - Publish ValidationResponse

### Phase 6: Caching & Optimization

**Goal**: Add decision caching for performance.

**Steps**:
1. Create `internal/cache/decision_cache.go`
   - Cache key: hash of (user, operation, data_providers, policy_version)
   - TTL: 60 seconds
   - Invalidate on policy update

## Configuration

### config.yaml

```yaml
service:
  name: policy-enforcer
  port: 8080  # For health checks/metrics

rabbitmq:
  url: amqp://guest:guest@localhost:5672/
  consumer:
    queue: policy-enforcer-requests
    routing_key: request.approval.*
    prefetch_count: 10
    auto_ack: false
  publisher:
    exchange: dynamos
    exchange_type: topic

eflint:
  servers:
    - host: localhost
      port: 8001
  pool:
    max_connections: 10
    max_idle: 5
    timeout: 5s
  policy:
    specification_path: /etc/policies/current.eflint
    auto_reload: false

cache:
  enabled: true
  ttl: 60s
  max_size: 10000

logging:
  level: info
  format: json
```

## Go Dependencies

```go
// go.mod
module github.com/your-org/policy-enforcer

go 1.21

require (
    github.com/rabbitmq/amqp091-go v1.9.0
    github.com/spf13/viper v1.18.2          // Config management
    go.uber.org/zap v1.26.0                 // Logging
    google.golang.org/protobuf v1.31.0      // Protobuf
)
```

## Example Code Snippets

### eFLINT Client

```go
package eflint

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "net"
    "sync"
)

type Client struct {
    addr   string
    conn   net.Conn
    mu     sync.Mutex
    reader *bufio.Reader
    writer *bufio.Writer
}

// Response matches the eFLINT server JSON response format
type Response struct {
    Response             string       `json:"response"`
    OldState             int          `json:"old-state"`
    NewState             int          `json:"new-state"`
    SourceContents       []Value      `json:"source_contents"`
    TargetContents       []Value      `json:"target_contents"`
    CreatedFacts         []Value      `json:"created_facts"`
    TerminatedFacts      []Value      `json:"terminated_facts"`
    Violations           []Violation  `json:"violations"`
    OutputEvents         []Event      `json:"output-events"`
    Errors               []Error      `json:"errors"`
    QueryResults         []bool       `json:"query-results"`
    NewDuties            []Value      `json:"new-duties"`
    NewEnabledTransitions  []Value    `json:"new-enabled-transitions"`
    NewDisabledTransitions []Value    `json:"new-disabled-transitions"`
    AllDuties            []Value      `json:"all-duties"`
    AllEnabledTransitions  []Value    `json:"all-enabled-transitions"`
    AllDisabledTransitions []Value    `json:"all-disabled-transitions"`
}

type Violation struct {
    Violation string `json:"violation"` // "trigger", "duty", "invariant"
    Value     Value  `json:"value,omitempty"`
    Invariant string `json:"invariant,omitempty"`
}

type Value struct {
    TaggedType string   `json:"tagged-type"`
    FactType   string   `json:"fact-type"`
    Value      interface{} `json:"value"` // Can be string, int, or []Value
    Textual    string   `json:"textual"`
}

type Event struct {
    // Define based on actual eFLINT output-events structure
}

type Error struct {
    // Define based on actual eFLINT errors structure
}

func NewClient(addr string) (*Client, error) {
    conn, err := net.Dial("tcp", addr)
    if err != nil {
        return nil, fmt.Errorf("failed to connect to eFLINT server: %w", err)
    }
    
    return &Client{
        addr:   addr,
        conn:   conn,
        reader: bufio.NewReader(conn),
        writer: bufio.NewWriter(conn),
    }, nil
}

func (c *Client) ExecuteAction(ctx context.Context, cmd ActionCommand) (*Response, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Serialize command
    data, err := json.Marshal(cmd)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal command: %w", err)
    }
    
    // Send command (newline-delimited JSON)
    _, err = c.writer.Write(append(data, '\n'))
    if err != nil {
        return nil, fmt.Errorf("failed to write command: %w", err)
    }
    
    err = c.writer.Flush()
    if err != nil {
        return nil, fmt.Errorf("failed to flush: %w", err)
    }
    
    // Read response
    line, err := c.reader.ReadBytes('\n')
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }
    
    // Parse response
    var resp Response
    err = json.Unmarshal(line, &resp)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal response: %w", err)
    }
    
    return &resp, nil
}

func (c *Client) QueryEnabled(ctx context.Context, query EnabledQuery) (*Response, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // Similar to ExecuteAction but for enabled queries
    data, err := json.Marshal(query)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal query: %w", err)
    }
    
    _, err = c.writer.Write(append(data, '\n'))
    if err != nil {
        return nil, fmt.Errorf("failed to write query: %w", err)
    }
    
    err = c.writer.Flush()
    if err != nil {
        return nil, fmt.Errorf("failed to flush: %w", err)
    }
    
    line, err := c.reader.ReadBytes('\n')
    if err != nil {
        return nil, fmt.Errorf("failed to read response: %w", err)
    }
    
    var resp Response
    err = json.Unmarshal(line, &resp)
    if err != nil {
        return nil, fmt.Errorf("failed to unmarshal response: %w", err)
    }
    
    return &resp, nil
}

func (c *Client) GetStatus(ctx context.Context) (*Response, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    cmd := map[string]string{"command": "status"}
    data, _ := json.Marshal(cmd)
    
    c.writer.Write(append(data, '\n'))
    c.writer.Flush()
    
    line, err := c.reader.ReadBytes('\n')
    if err != nil {
        return nil, err
    }
    
    var resp Response
    json.Unmarshal(line, &resp)
    return &resp, nil
}

func (c *Client) Close() error {
    return c.conn.Close()
}

// Helper to check if request was approved
func (r *Response) IsApproved() bool {
    return r.Response == "success" && len(r.Violations) == 0
}

// Helper to get violation reasons
func (r *Response) GetViolationReasons() []string {
    var reasons []string
    for _, v := range r.Violations {
        if v.Value.Textual != "" {
            reasons = append(reasons, fmt.Sprintf("%s: %s", v.Violation, v.Value.Textual))
        } else if v.Invariant != "" {
            reasons = append(reasons, fmt.Sprintf("invariant violated: %s", v.Invariant))
        }
    }
    return reasons
}
```

### Request Translator

```go
package eflint

import (
    "fmt"
    pb "github.com/Jorrit05/DYNAMOS/pkg/proto"
)

// TranslateRequest converts a DYNAMOS RequestApproval to eFLINT action commands
// for each data provider specified in the request
func TranslateRequest(req *pb.RequestApproval) ([]ActionCommand, error) {
    var commands []ActionCommand
    
    // The eFLINT model uses submit-request action with:
    // Actor: requester (user email)
    // Recipient: organization (data provider)
    // Objects: [request-type, data-set, archetype, compute-provider]
    
    // For now, we'll use defaults for archetype and compute-provider
    // These would ideally come from request metadata or options
    archetype := determineArchetype(req)
    computeProvider := "SURF" // Default TTP
    dataset := "wageGap" // This should come from request metadata
    
    // Create one command per data provider
    for _, provider := range req.DataProviders {
        cmd := ActionCommand{
            Command:   "action",
            ActType:   "submit-request",
            Actor:     req.User.UserName,
            Recipient: provider,
            Objects: []string{
                req.Type,           // request-type: "sqlDataRequest"
                dataset,            // data-set: "wageGap"
                archetype,          // archetype: "computeToData" or "dataThroughTtp"
                computeProvider,    // compute-provider: "SURF"
            },
            Force: "false",
        }
        commands = append(commands, cmd)
    }
    
    return commands, nil
}

func determineArchetype(req *pb.RequestApproval) string {
    // Check options to determine archetype
    if req.Options != nil {
        if req.Options["aggregate"] {
            return "dataThroughTtp" // Need to aggregate at TTP
        }
    }
    return "computeToData" // Default: compute at each provider
}

// Alternative: Query if action is enabled before executing
func BuildEnabledQuery(req *pb.RequestApproval, provider string) EnabledQuery {
    archetype := determineArchetype(req)
    computeProvider := "SURF"
    dataset := "wageGap"
    
    return EnabledQuery{
        Command: "enabled",
        Value: CompositeValue{
            TaggedType: "Act",
            FactType:   "submit-request",
            Value: []AtomValue{
                {TaggedType: "String", FactType: "requester", Value: req.User.UserName},
                {TaggedType: "String", FactType: "organization", Value: provider},
                {TaggedType: "String", FactType: "request-type", Value: req.Type},
                {TaggedType: "String", FactType: "data-set", Value: dataset},
                {TaggedType: "String", FactType: "archetype", Value: archetype},
                {TaggedType: "String", FactType: "compute-provider", Value: computeProvider},
            },
        },
    }
}

// ActionCommand for executing an action
type ActionCommand struct {
    Command   string   `json:"command"`
    ActType   string   `json:"act-type"`
    Actor     string   `json:"actor"`
    Recipient string   `json:"recipient"`
    Objects   []string `json:"objects"`
    Force     string   `json:"force"`
}

// EnabledQuery for checking if action is enabled
type EnabledQuery struct {
    Command string         `json:"command"`
    Value   CompositeValue `json:"value"`
}

type CompositeValue struct {
    TaggedType string      `json:"tagged-type"`
    FactType   string      `json:"fact-type"`
    Value      []AtomValue `json:"value"`
    Textual    string      `json:"textual,omitempty"`
}

type AtomValue struct {
    TaggedType string `json:"tagged-type"`
    FactType   string `json:"fact-type"`
    Value      string `json:"value"`
    Textual    string `json:"textual,omitempty"`
}
```

### Request Handler

```go
package handler

import (
    "context"
    "fmt"
    
    pb "github.com/Jorrit05/DYNAMOS/pkg/proto"
    "github.com/your-org/policy-enforcer/internal/eflint"
    "github.com/your-org/policy-enforcer/internal/cache"
)

type RequestHandler struct {
    eflintClient *eflint.Client
    cache        *cache.DecisionCache
}

func NewRequestHandler(eflintClient *eflint.Client, cache *cache.DecisionCache) *RequestHandler {
    return &RequestHandler{
        eflintClient: eflintClient,
        cache:        cache,
    }
}

func (h *RequestHandler) HandleRequestApproval(
    ctx context.Context,
    req *pb.RequestApproval,
) (*pb.ValidationResponse, error) {
    
    // 1. Check cache
    cacheKey := h.buildCacheKey(req)
    if cached := h.cache.Get(cacheKey); cached != nil {
        return cached, nil
    }
    
    // 2. Translate to eFLINT commands
    commands := eflint.TranslateRequest(req)
    
    // 3. Execute commands sequentially
    var finalResult *eflint.Response
    for _, cmd := range commands {
        result, err := h.eflintClient.Execute(ctx, cmd)
        if err != nil {
            return nil, fmt.Errorf("eFLINT execution error: %w", err)
        }
        finalResult = result
    }
    
    // 4. Parse result and build response
    response := h.buildValidationResponse(req, finalResult)
    
    // 5. Cache the decision
    h.cache.Set(cacheKey, response)
    
    return response, nil
}

func (h *RequestHandler) buildValidationResponse(
    req *pb.RequestApproval,
    eflintResult *eflint.Response,
) *pb.ValidationResponse {
    
    response := &pb.ValidationResponse{
        Type:            "ValidationResponse",
        RequestType:     req.Type,
        User:            req.User,
        RequestApproved: eflintResult.Enabled,
        PolicyVersion:   "v1.0.0", // Get from config or eFLINT state
    }
    
    if !eflintResult.Enabled {
        // Request denied
        response.DenialReason = eflintResult.Reason
        response.ViolatedNorms = eflintResult.ViolatedNorms
        response.InvalidDataproviders = req.DataProviders
    } else {
        // Request approved
        response.ValidArchetypes = h.determineValidArchetypes(req)
        response.ValidDataproviders = h.buildValidProviders(req)
        response.Options = req.Options
    }
    
    return response
}

func (h *RequestHandler) determineValidArchetypes(req *pb.RequestApproval) *pb.UserArchetypes {
    // Determine which archetypes are allowed based on policy
    // For now, return both if approved
    return &pb.UserArchetypes{
        UserName: req.User.UserName,
        Archetypes: map[string]*pb.UserAllowedArchetypes{
            req.User.UserName: {
                Archetypes: []string{"ComputeToData", "DataThroughTtp"},
            },
        },
    }
}

func (h *RequestHandler) buildValidProviders(req *pb.RequestApproval) map[string]*pb.DataProvider {
    providers := make(map[string]*pb.DataProvider)
    
    for _, providerName := range req.DataProviders {
        providers[providerName] = &pb.DataProvider{
            Archetypes:       []string{"ComputeToData", "DataThroughTtp"},
            ComputeProviders: []string{"surf"}, // TTP
        }
    }
    
    return providers
}

func (h *RequestHandler) buildCacheKey(req *pb.RequestApproval) string {
    // Simple cache key - in production use proper hashing
    return fmt.Sprintf("%s:%s:%v",
        req.User.UserName,
        req.OperationType,
        req.DataProviders,
    )
}
```

## Testing Strategy

### Unit Tests
- Test eFLINT client with mock TCP server
- Test translator with sample RequestApproval messages
- Test parser with sample eFLINT responses

### Integration Tests
- Test full flow with real eFLINT server
- Test RabbitMQ message handling
- Test error scenarios

### Manual Testing
1. Start eFLINT server: `eflint-server --port 8001`
2. Start RabbitMQ: `docker run -p 5672:5672 rabbitmq:3-management`
3. Send test RequestApproval to RabbitMQ
4. Verify ValidationResponse is published

## Common Issues & Solutions

### Issue 1: eFLINT Connection Refused
**Solution**: Ensure eFLINT server is running and port is correct

### Issue 2: Protobuf Deserialization Errors
**Solution**: Ensure proto files are synced with DYNAMOS repo

### Issue 3: RabbitMQ Connection Lost
**Solution**: Implement reconnection logic with exponential backoff

### Issue 4: Policy Not Loaded
**Solution**: Send load command to eFLINT before processing requests

## Performance Considerations

1. **Connection Pooling**: Maintain 5-10 connections to eFLINT server
2. **Caching**: Cache decisions for 60 seconds with policy version as key
3. **Async Processing**: Use goroutines for concurrent request handling
4. **Metrics**: Track validation latency (target: <50ms p95)

## Next Steps After Basic Implementation

1. **Dynamic Policy Updates**: Listen for PolicyUpdate messages
2. **Hot Reload**: Reload eFLINT specifications without restart
3. **State Management**: Export/import eFLINT state for persistence
4. **Monitoring**: Add Prometheus metrics
5. **Tracing**: Add OpenTelemetry/Jaeger tracing

## References

- DYNAMOS Repo: https://github.com/Jorrit05/DYNAMOS
- eFLINT Haskell Implementation: https://gitlab.com/eflint/haskell-implementation
- eFLINT Language Spec: See Zotero papers
- DYNAMOS Paper: See artifacts for full context

## Questions to Answer During Development

1. **Policy Loading**: Where is the eFLINT specification stored initially? Config file or etcd?
2. **Archetype Mapping**: Is archetype selection logic in eFLINT spec or in Policy Enforcer?
3. **Error Handling**: What's the fail-safe behavior? Deny by default?
4. **State Persistence**: Should eFLINT state be periodically backed up to etcd via Orchestrator?