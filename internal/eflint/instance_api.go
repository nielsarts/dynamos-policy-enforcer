package eflint

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------
// Instance API Handler
// -----------------------------------------------------------------------------

// InstanceAPIHandler handles HTTP requests for eFLINT instance lifecycle management.
// It provides endpoints for starting, stopping, and sending commands to the eFLINT server.
type InstanceAPIHandler struct {
	manager *Manager
	logger  *zap.Logger
}

// NewInstanceAPIHandler creates a new instance API handler with the given manager and logger.
func NewInstanceAPIHandler(manager *Manager, logger *zap.Logger) *InstanceAPIHandler {
	return &InstanceAPIHandler{
		manager: manager,
		logger:  logger,
	}
}

// RegisterRoutes registers all instance management API routes on the given Echo group.
// Routes are registered under the group prefix (e.g., /eflint).
func (h *InstanceAPIHandler) RegisterRoutes(g *echo.Group) {
	g.GET("/status", h.GetStatus)
	g.POST("/start", h.Start)
	g.POST("/stop", h.Stop)
	g.POST("/command", h.SendCommand)
}

// -----------------------------------------------------------------------------
// Request/Response Types
// -----------------------------------------------------------------------------

// StatusResponse represents the response for status-related endpoints.
type StatusResponse struct {
	Running       bool   `json:"running"`                  // Whether the instance is running
	Port          int    `json:"port,omitempty"`           // The port the instance is listening on
	ModelLocation string `json:"model_location,omitempty"` // Path to the loaded model
}

// StartRequest represents the request body for starting an instance.
type StartRequest struct {
	ModelLocation string `json:"model_location" validate:"required"` // Path to the eFLINT model file
	Force         bool   `json:"force,omitempty"`                    // Force restart if already running
}

// CommandRequest represents the request body for sending a command.
type CommandRequest struct {
	Command string `json:"command" validate:"required"` // The JSON command to send to eFLINT
}

// CommandResponse represents the response from a command execution.
type CommandResponse struct {
	Parsed json.RawMessage `json:"response"` // The parsed JSON response from eFLINT
}

// ErrorResponse represents an error response returned by the API.
type ErrorResponse struct {
	Error string `json:"error"` // Human-readable error message
}

// -----------------------------------------------------------------------------
// Utility Functions
// -----------------------------------------------------------------------------

// mustMarshal marshals a value to JSON, returning empty bytes on error.
// Used for simple string wrapping in error cases.
func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// -----------------------------------------------------------------------------
// Handler Methods
// -----------------------------------------------------------------------------

// GetStatus returns the current status of the eFLINT instance.
// GET /eflint/status
func (h *InstanceAPIHandler) GetStatus(c echo.Context) error {
	status := h.manager.Status()

	return c.JSON(http.StatusOK, StatusResponse{
		Running:       status.Running,
		Port:          status.Port,
		ModelLocation: status.ModelLocation,
	})
}

// Start starts the eFLINT instance with the given model.
// If an instance is already running and force=false, returns a conflict error.
// If force=true, the existing instance is stopped and a new one is started.
// POST /eflint/start
func (h *InstanceAPIHandler) Start(c echo.Context) error {
	var req StartRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.ModelLocation == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "model_location is required"})
	}

	// Check if instance is already running
	if h.manager.IsRunning() && !req.Force {
		return c.JSON(http.StatusConflict, ErrorResponse{Error: "instance already running, use force=true to restart"})
	}

	if err := h.manager.Start(req.ModelLocation); err != nil {
		h.logger.Error("failed to start instance", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	status := h.manager.Status()
	return c.JSON(http.StatusOK, StatusResponse{
		Running:       status.Running,
		Port:          status.Port,
		ModelLocation: status.ModelLocation,
	})
}

// Stop stops the running eFLINT instance.
// POST /eflint/stop
func (h *InstanceAPIHandler) Stop(c echo.Context) error {
	if err := h.manager.Stop(); err != nil {
		if err == ErrInstanceNotFound {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no instance running"})
		}
		h.logger.Error("failed to stop instance", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, StatusResponse{Running: false})
}

// SendCommand sends a command to the eFLINT instance.
// POST /eflint/command
func (h *InstanceAPIHandler) SendCommand(c echo.Context) error {
	var req CommandRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.Command == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "command is required"})
	}

	response, err := h.manager.SendCommand(req.Command)
	if err != nil {
		if err == ErrInstanceNotFound {
			return c.JSON(http.StatusNotFound, ErrorResponse{Error: "no instance running"})
		}
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}
		h.logger.Error("failed to send command", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Parse the response as JSON
	var parsed json.RawMessage
	if json.Valid([]byte(response)) {
		parsed = json.RawMessage(response)
	} else {
		parsed = json.RawMessage(`{"raw": ` + string(mustMarshal(response)) + `}`)
	}

	return c.JSON(http.StatusOK, CommandResponse{
		Parsed: parsed,
	})
}
