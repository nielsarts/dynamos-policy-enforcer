package policyenforcer

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// -----------------------------------------------------------------------------
// HTTP Handler
// -----------------------------------------------------------------------------

// HTTPHandler handles HTTP requests for the policy enforcer API.
// It provides REST endpoints for querying allowed clauses and validating requests.
type HTTPHandler struct {
	enforcer *Enforcer
	logger   *zap.Logger
}

// NewHTTPHandler creates a new HTTP handler for the policy enforcer.
func NewHTTPHandler(enforcer *Enforcer, logger *zap.Logger) *HTTPHandler {
	return &HTTPHandler{
		enforcer: enforcer,
		logger:   logger,
	}
}

// RegisterRoutes registers all policy enforcer API routes on the given Echo group.
// Routes are registered under the group prefix (e.g., /policy-enforcer).
func (h *HTTPHandler) RegisterRoutes(g *echo.Group) {
	// Reasoner info
	g.GET("/info", h.GetReasonerInfo)

	// Allowed clauses endpoints
	g.GET("/allowed-request-types", h.GetAllowedRequestTypes)
	g.GET("/allowed-data-sets", h.GetAllowedDataSets)
	g.GET("/allowed-archetypes", h.GetAllowedArchetypes)
	g.GET("/allowed-compute-providers", h.GetAllowedComputeProviders)
	g.GET("/allowed-clauses", h.GetAllAllowedClauses)

	// Request validation endpoint
	g.POST("/validate", h.ValidateRequest)

	// Availability endpoints (organization-level, not requester-specific)
	g.GET("/available-archetypes", h.GetAvailableArchetypes)
	g.GET("/available-compute-providers", h.GetAvailableComputeProviders)
}

// -----------------------------------------------------------------------------
// Handler Methods
// -----------------------------------------------------------------------------

// GetReasonerInfo returns information about the active reasoner.
// GET /policy-enforcer/info
func (h *HTTPHandler) GetReasonerInfo(c echo.Context) error {
	info := h.enforcer.GetReasonerInfo()
	return c.JSON(http.StatusOK, info)
}

// GetAllowedRequestTypes returns all request types allowed for a requester at an organization.
// GET /policy-enforcer/allowed-request-types?organization=VU&requester=user@example.com
func (h *HTTPHandler) GetAllowedRequestTypes(c echo.Context) error {
	organization, requester, err := h.parseOrgRequester(c)
	if err != nil {
		return err
	}

	result, err := h.enforcer.GetAllowedRequestTypes(c.Request().Context(), organization, requester)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// GetAllowedDataSets returns all datasets allowed for a requester at an organization.
// GET /policy-enforcer/allowed-data-sets?organization=VU&requester=user@example.com
func (h *HTTPHandler) GetAllowedDataSets(c echo.Context) error {
	organization, requester, err := h.parseOrgRequester(c)
	if err != nil {
		return err
	}

	result, err := h.enforcer.GetAllowedDataSets(c.Request().Context(), organization, requester)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// GetAllowedArchetypes returns all archetypes allowed for a requester at an organization.
// GET /policy-enforcer/allowed-archetypes?organization=VU&requester=user@example.com
func (h *HTTPHandler) GetAllowedArchetypes(c echo.Context) error {
	organization, requester, err := h.parseOrgRequester(c)
	if err != nil {
		return err
	}

	result, err := h.enforcer.GetAllowedArchetypes(c.Request().Context(), organization, requester)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// GetAllowedComputeProviders returns all compute providers allowed for a requester at an organization.
// GET /policy-enforcer/allowed-compute-providers?organization=VU&requester=user@example.com
func (h *HTTPHandler) GetAllowedComputeProviders(c echo.Context) error {
	organization, requester, err := h.parseOrgRequester(c)
	if err != nil {
		return err
	}

	result, err := h.enforcer.GetAllowedComputeProviders(c.Request().Context(), organization, requester)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// GetAllAllowedClauses returns all allowed clauses for a requester at an organization.
// GET /policy-enforcer/allowed-clauses?organization=VU&requester=user@example.com
func (h *HTTPHandler) GetAllAllowedClauses(c echo.Context) error {
	organization, requester, err := h.parseOrgRequester(c)
	if err != nil {
		return err
	}

	result, err := h.enforcer.GetAllAllowedClauses(c.Request().Context(), organization, requester)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// ValidateRequest checks if a specific request is allowed.
// POST /policy-enforcer/validate
// Body: { "organization": "VU", "requester": "user@example.com", "request_type": "sqlDataRequest", ... }
func (h *HTTPHandler) ValidateRequest(c echo.Context) error {
	var params ValidateRequestParams
	if err := c.Bind(&params); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
	}

	// Validate required fields
	if params.Organization == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "organization is required"})
	}
	if params.Requester == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "requester is required"})
	}
	if params.RequestType == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "request_type is required"})
	}
	if params.DataSet == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "data_set is required"})
	}
	if params.Archetype == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "archetype is required"})
	}
	if params.ComputeProvider == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "compute_provider is required"})
	}

	result, err := h.enforcer.ValidateRequest(c.Request().Context(), &params)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, result)
}

// GetAvailableArchetypes returns archetypes available at an organization (not requester-specific).
// GET /policy-enforcer/available-archetypes?organization=VU
func (h *HTTPHandler) GetAvailableArchetypes(c echo.Context) error {
	organization := c.QueryParam("organization")
	if organization == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "organization parameter is required"})
	}

	values, err := h.enforcer.GetAvailableArchetypes(c.Request().Context(), organization)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"organization": organization,
		"archetypes":   values,
	})
}

// GetAvailableComputeProviders returns compute providers available at an organization (not requester-specific).
// GET /policy-enforcer/available-compute-providers?organization=VU
func (h *HTTPHandler) GetAvailableComputeProviders(c echo.Context) error {
	organization := c.QueryParam("organization")
	if organization == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{Error: "organization parameter is required"})
	}

	values, err := h.enforcer.GetAvailableComputeProviders(c.Request().Context(), organization)
	if err != nil {
		return h.handleError(c, err)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"organization":      organization,
		"compute_providers": values,
	})
}

// -----------------------------------------------------------------------------
// Helper Methods
// -----------------------------------------------------------------------------

// parseOrgRequester extracts and validates organization and requester query parameters.
func (h *HTTPHandler) parseOrgRequester(c echo.Context) (organization, requester string, err error) {
	organization = c.QueryParam("organization")
	requester = c.QueryParam("requester")

	if organization == "" {
		return "", "", c.JSON(http.StatusBadRequest, ErrorResponse{Error: "organization parameter is required"})
	}
	if requester == "" {
		return "", "", c.JSON(http.StatusBadRequest, ErrorResponse{Error: "requester parameter is required"})
	}

	return organization, requester, nil
}

// handleError converts service errors to appropriate HTTP responses.
func (h *HTTPHandler) handleError(c echo.Context, err error) error {
	// Check if reasoner is not running
	if !h.enforcer.IsRunning() {
		return c.JSON(http.StatusServiceUnavailable, ErrorResponse{Error: "reasoner is not running"})
	}

	h.logger.Error("policy enforcer error", zap.Error(err))
	return c.JSON(http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
}
