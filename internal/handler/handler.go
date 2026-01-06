// Package handler provides message handling functionality for processing
// policy validation requests via RabbitMQ.
package handler

import (
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.uber.org/zap"

	"github.com/nielsarts/dynamos-policy-enforcer/internal/eflint"
)

// Handler processes incoming RequestApproval messages from RabbitMQ
// and validates them against the eFLINT policy engine.
type Handler struct {
	manager *eflint.Manager
	logger  *zap.Logger
}

// RequestApproval represents an incoming policy validation request message.
type RequestApproval struct {
	RequestID string                 `json:"request_id"`        // Unique identifier for the request
	Action    string                 `json:"action"`            // The action being requested
	Resource  string                 `json:"resource"`          // The resource being accessed
	Principal string                 `json:"principal"`         // The entity making the request
	Context   map[string]interface{} `json:"context,omitempty"` // Additional context data
	Timestamp string                 `json:"timestamp"`         // When the request was created
}

// ValidationResponse represents the response to a policy validation request.
type ValidationResponse struct {
	RequestID string `json:"request_id"`       // Matches the request's ID
	Approved  bool   `json:"approved"`         // Whether the action is allowed
	Reason    string `json:"reason,omitempty"` // Explanation for the decision
	Timestamp string `json:"timestamp"`        // When the response was generated
}

// NewHandler creates a new request handler with the given eFLINT manager.
func NewHandler(manager *eflint.Manager, logger *zap.Logger) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
	}
}

// Handle processes a single RabbitMQ message containing a policy validation request.
// It parses the request, queries the eFLINT engine, and sends a response.
func (h *Handler) Handle(msg amqp.Delivery) error {
	h.logger.Info("received message", zap.String("correlation_id", msg.CorrelationId))

	// Parse request
	var request RequestApproval
	if err := json.Unmarshal(msg.Body, &request); err != nil {
		h.logger.Error("failed to unmarshal request", zap.Error(err))
		msg.Nack(false, false) // Don't requeue invalid messages
		return fmt.Errorf("invalid message format: %w", err)
	}

	h.logger.Info("processing request",
		zap.String("request_id", request.RequestID),
		zap.String("action", request.Action),
		zap.String("resource", request.Resource),
		zap.String("principal", request.Principal),
	)

	// Query eFLINT server
	approved, reason, err := h.queryEFlint(request)
	if err != nil {
		h.logger.Error("failed to query eFLINT", zap.Error(err))
		msg.Nack(false, true) // Requeue on error
		return err
	}

	// Send response
	response := ValidationResponse{
		RequestID: request.RequestID,
		Approved:  approved,
		Reason:    reason,
		Timestamp: request.Timestamp,
	}

	if err := h.sendResponse(msg, response); err != nil {
		return err
	}

	msg.Ack(false)
	h.logger.Info("successfully processed request",
		zap.String("request_id", request.RequestID),
		zap.Bool("approved", approved),
	)

	return nil
}

// queryEFlint sends a query to the eFLINT server and parses the response.
// Returns whether the action is approved, the reason, and any error.
func (h *Handler) queryEFlint(request RequestApproval) (bool, string, error) {
	// Build the eFLINT query command
	queryData := map[string]interface{}{
		"action":    request.Action,
		"resource":  request.Resource,
		"principal": request.Principal,
	}
	if request.Context != nil {
		queryData["context"] = request.Context
	}

	cmdJSON, err := json.Marshal(map[string]interface{}{
		"command": "query",
		"data":    queryData,
	})
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal command: %w", err)
	}

	// Send command via manager
	resp, err := h.manager.SendCommand(string(cmdJSON))
	if err != nil {
		return false, "", fmt.Errorf("failed to send command to eFLINT: %w", err)
	}

	// Parse JSON response
	var respData map[string]interface{}
	if err := json.Unmarshal([]byte(resp), &respData); err != nil {
		return false, "", fmt.Errorf("failed to parse eFLINT response: %w", err)
	}

	// Extract decision from response
	approved := false
	if val, ok := respData["approved"]; ok {
		if boolVal, ok := val.(bool); ok {
			approved = boolVal
		}
	}

	reason := ""
	if val, ok := respData["reason"]; ok {
		if strVal, ok := val.(string); ok {
			reason = strVal
		}
	}
	if val, ok := respData["message"]; ok {
		if strVal, ok := val.(string); ok && reason == "" {
			reason = strVal
		}
	}

	return approved, reason, nil
}

// sendResponse publishes a response message back to RabbitMQ.
// Currently logs the response; in production, this would publish to a response queue.
func (h *Handler) sendResponse(msg amqp.Delivery, response ValidationResponse) error {
	responseJSON, err := json.Marshal(response)
	if err != nil {
		h.logger.Error("failed to marshal response", zap.Error(err))
		msg.Nack(false, false)
		return fmt.Errorf("failed to marshal response: %w", err)
	}

	// TODO: In production, publish this to a response queue
	// For now, just log the response
	h.logger.Info("response generated", zap.String("response", string(responseJSON)))

	return nil
}
