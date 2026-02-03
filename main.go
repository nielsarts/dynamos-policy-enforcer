// Package main provides the entry point for the DYNAMOS Policy Enforcer service.
// This service validates data requests against policy rules defined in a reasoning engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	"github.com/nielsarts/dynamos-policy-enforcer/internal/config"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/eflint"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/policyenforcer"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/reasoner"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to config file")
	httpPort := flag.Int("port", 8080, "HTTP server port")
	autoStart := flag.Bool("auto-start", true, "Auto-start eFLINT with model from config")
	flag.Parse()

	// Initialize logger
	logger, err := initLogger()
	if err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Warn("failed to load config, using defaults", zap.Error(err))
		cfg = &config.Config{
			EFlint: config.EFlintConfig{
				ServerPath: "eflint-server",
				ModelPath:  "eflint/dynamos-agreement.eflint",
				Timeout:    60 * time.Second,
			},
		}
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Health check endpoint
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "healthy"})
	})

	// Initialize eFLINT Manager
	managerConfig := &eflint.ManagerConfig{
		EflintServerPath:  cfg.EFlint.ServerPath,
		MinPort:           1025,
		MaxPort:           65535,
		StartupDelay:      3 * time.Second,
		ConnectionTimeout: cfg.EFlint.Timeout,
	}
	manager := eflint.NewManager(managerConfig, logger)

	// Initialize StateManager for checkpointing (POC)
	stateManager := eflint.NewStateManager(manager, "eflint-states", logger)

	// -----------------------------------------------------------------------------
	// eFLINT API Group - Low-level eFLINT server management
	// These endpoints provide direct access to the eFLINT reasoner
	// -----------------------------------------------------------------------------
	eflintGroup := e.Group("/eflint")

	// Instance management API
	instanceAPIHandler := eflint.NewInstanceAPIHandler(manager, logger)
	instanceAPIHandler.RegisterRoutes(eflintGroup)

	// State management API (POC)
	stateAPIHandler := eflint.NewStateAPIHandler(stateManager, logger)
	stateAPIHandler.RegisterRoutes(eflintGroup)

	// -----------------------------------------------------------------------------
	// Policy Enforcer API Group - High-level policy enforcement
	// These endpoints provide a reasoner-agnostic interface for policy validation
	// In the future, this could work with Symboleo, JSON-based agreements, etc.
	// -----------------------------------------------------------------------------

	// Create the eFLINT reasoner (implements the Reasoner interface)
	eflintReasoner := reasoner.NewEflintReasoner(manager, logger)

	// Create the policy enforcer (uses the Reasoner interface)
	enforcer := policyenforcer.NewEnforcer(eflintReasoner, logger)

	// Register HTTP handlers for policy enforcer
	policyEnforcerGroup := e.Group("/policy-enforcer")
	policyEnforcerHandler := policyenforcer.NewHTTPHandler(enforcer, logger)
	policyEnforcerHandler.RegisterRoutes(policyEnforcerGroup)

	// -----------------------------------------------------------------------------
	// Auto-start eFLINT if configured
	// -----------------------------------------------------------------------------
	if *autoStart && cfg.EFlint.ModelPath != "" {
		logger.Info("auto-starting eFLINT server",
			zap.String("model", cfg.EFlint.ModelPath),
		)
		if err := manager.Start(cfg.EFlint.ModelPath); err != nil {
			logger.Error("failed to auto-start eFLINT server", zap.Error(err))
			// Continue anyway - the server can be started manually via API
		}
	}

	// -----------------------------------------------------------------------------
	// Start HTTP Server
	// -----------------------------------------------------------------------------
	go func() {
		addr := fmt.Sprintf(":%d", *httpPort)
		logger.Info("starting HTTP server",
			zap.String("address", addr),
		)
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start HTTP server", zap.Error(err))
		}
	}()

	// -----------------------------------------------------------------------------
	// Graceful Shutdown
	// -----------------------------------------------------------------------------
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down...")

	// Stop eFLINT server
	if manager.IsRunning() {
		if err := manager.Stop(); err != nil {
			logger.Error("failed to stop eFLINT server", zap.Error(err))
		}
	}

	// Shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown HTTP server", zap.Error(err))
	}

	logger.Info("shutdown complete")
}

// initLogger initializes the zap logger.
func initLogger() (*zap.Logger, error) {
	// Check if we're in development mode
	if os.Getenv("PE_LOGGING_DEVELOPMENT") == "true" {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}
