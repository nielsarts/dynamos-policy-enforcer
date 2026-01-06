package eflint

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------
// State API Handler
// -----------------------------------------------------------------------------

// StateAPIHandler handles HTTP requests for eFLINT state management.
// This is a POC (Proof of Concept) for stateful session management with
// state persistence, allowing export/import of eFLINT execution graphs.
type StateAPIHandler struct {
	stateManager *StateManager
	logger       *zap.Logger
}

// NewStateAPIHandler creates a new StateAPIHandler with the given manager and logger.
func NewStateAPIHandler(stateManager *StateManager, logger *zap.Logger) *StateAPIHandler {
	return &StateAPIHandler{
		stateManager: stateManager,
		logger:       logger,
	}
}

// RegisterRoutes registers all state management API routes on the given Echo group.
// Routes are registered under the group prefix (e.g., /eflint/state).
func (h *StateAPIHandler) RegisterRoutes(g *echo.Group) {
	g.GET("", h.GetState)            // GET /eflint/state - get current state
	g.POST("/export", h.ExportState) // POST /eflint/state/export - export for persistence
	g.POST("/import", h.ImportState) // POST /eflint/state/import - import saved state
	g.POST("/checkpoint", h.CreateCheckpoint)
	g.POST("/checkpoint/restore", h.RestoreCheckpoint)
	g.GET("/checkpoints", h.ListCheckpoints)
	g.DELETE("/checkpoint/:name", h.DeleteCheckpoint)
}

// -----------------------------------------------------------------------------
// Request/Response Types
// -----------------------------------------------------------------------------

// ExportStateResponse represents the response for state export.
type ExportStateResponse struct {
	Success bool        `json:"success"`         // Whether the export was successful
	State   *SavedState `json:"state,omitempty"` // The exported state data
	Error   string      `json:"error,omitempty"` // Error message if failed
}

// ImportStateRequest represents the request body for importing state.
type ImportStateRequest struct {
	State *SavedState `json:"state" validate:"required"` // The state to import
}

// CheckpointRequest represents a request for checkpoint operations.
type CheckpointRequest struct {
	Name string `json:"name" validate:"required"` // Name of the checkpoint
}

// CheckpointListResponse represents the list of available checkpoints.
type CheckpointListResponse struct {
	Checkpoints []string `json:"checkpoints"` // List of checkpoint names
}

// SuccessResponse represents a generic success response.
type SuccessResponse struct {
	Success bool        `json:"success"`           // Whether the operation succeeded
	Message string      `json:"message,omitempty"` // Optional success message
	Data    interface{} `json:"data,omitempty"`    // Optional additional data
}

// StateResponse represents the response for the GetState endpoint.
type StateResponse struct {
	State json.RawMessage `json:"state"` // The current execution graph state
}

// -----------------------------------------------------------------------------
// Handler Methods
// -----------------------------------------------------------------------------

// GetState retrieves the current execution graph state of the eFLINT instance.
// GET /eflint/state
func (h *StateAPIHandler) GetState(c echo.Context) error {
	response, err := h.stateManager.GetState()
	if err != nil {
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}
		h.logger.Error("failed to get state", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Parse the response as JSON
	var state json.RawMessage
	if json.Valid([]byte(response)) {
		state = json.RawMessage(response)
	} else {
		state = json.RawMessage(`{"raw": "` + response + `"}`)
	}

	return c.JSON(http.StatusOK, StateResponse{
		State: state,
	})
}

// ExportState exports the current eFLINT state for persistence.
// POST /eflint/state/export
func (h *StateAPIHandler) ExportState(c echo.Context) error {
	state, err := h.stateManager.ExportState()
	if err != nil {
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}
		h.logger.Error("failed to export state", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, ExportStateResponse{
		Success: true,
		State:   state,
	})
}

// ImportState imports a previously exported state.
// POST /eflint/state/import
func (h *StateAPIHandler) ImportState(c echo.Context) error {
	var req ImportStateRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.State == nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "state is required"})
	}

	if err := h.stateManager.ImportState(req.State); err != nil {
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}
		h.logger.Error("failed to import state", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "state imported successfully",
	})
}

// CreateCheckpoint creates a named checkpoint of the current state
// POST /eflint/state/checkpoint
func (h *StateAPIHandler) CreateCheckpoint(c echo.Context) error {
	var req CheckpointRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	state, err := h.stateManager.CreateCheckpoint(req.Name)
	if err != nil {
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}
		h.logger.Error("failed to create checkpoint", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":    true,
		"checkpoint": req.Name,
		"state_id":   state.ID,
		"saved_at":   state.SavedAt,
	})
}

// RestoreCheckpoint restores a previously created checkpoint
// POST /eflint/state/checkpoint/restore
// NOTE: Due to a bug in the eFLINT server, full state restoration may not work.
// In that case, the instance will be restarted to the initial model state.
func (h *StateAPIHandler) RestoreCheckpoint(c echo.Context) error {
	var req CheckpointRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	if err := h.stateManager.RestoreCheckpoint(req.Name); err != nil {
		if err == ErrInstanceNotRunning {
			return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "instance is not running"})
		}

		// Check if the error indicates the instance was restarted
		errStr := err.Error()
		if strings.Contains(errStr, "restarted to initial state") {
			h.logger.Warn("checkpoint restore failed, instance restarted to initial state", zap.Error(err))
			return c.JSON(http.StatusOK, map[string]interface{}{
				"success":  false,
				"warning":  "eFLINT server does not support load-export; instance was restarted to initial model state instead",
				"restored": "initial",
				"note":     "This is a limitation of the eFLINT server's load-export functionality",
			})
		}

		h.logger.Error("failed to restore checkpoint", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success":  true,
		"message":  "checkpoint restored successfully",
		"restored": req.Name,
	})
}

// ListCheckpoints lists all available checkpoints
// GET /eflint/state/checkpoints
func (h *StateAPIHandler) ListCheckpoints(c echo.Context) error {
	states, err := h.stateManager.ListSavedStates()
	if err != nil {
		h.logger.Error("failed to list checkpoints", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	// Filter only checkpoints
	var checkpoints []string
	for _, s := range states {
		if len(s) > 11 && s[:11] == "checkpoint-" {
			checkpoints = append(checkpoints, s[11:])
		}
	}

	return c.JSON(http.StatusOK, CheckpointListResponse{
		Checkpoints: checkpoints,
	})
}

// DeleteCheckpoint deletes a checkpoint
// DELETE /eflint/state/checkpoint/:name
func (h *StateAPIHandler) DeleteCheckpoint(c echo.Context) error {
	name := c.Param("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "name is required"})
	}

	if err := h.stateManager.DeleteSavedState("checkpoint-" + name); err != nil {
		h.logger.Error("failed to delete checkpoint", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "checkpoint deleted successfully",
		"deleted": name,
	})
}
