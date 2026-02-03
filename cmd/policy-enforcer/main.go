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
	"go.uber.org/zap/zapcore"

	"github.com/nielsarts/dynamos-policy-enforcer/internal/config"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/eflint"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/policyenforcer"
	"github.com/nielsarts/dynamos-policy-enforcer/internal/reasoner"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "./configs/config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg.Logging)
	defer logger.Sync()

	logger.Info("starting Policy Enforcer",
		zap.String("config", *configPath),
		zap.String("version", "0.1.0"),
	)

	// Initialize eFLINT manager
	eflintConfig := &eflint.ManagerConfig{
		EflintServerPath:  cfg.EFlint.ServerPath,
		MinPort:           1025,
		MaxPort:           65535,
		StartupDelay:      3 * time.Second,
		ConnectionTimeout: cfg.EFlint.Timeout,
	}
	eflintManager := eflint.NewManager(eflintConfig, logger)
	logger.Info("eFLINT manager initialized",
		zap.String("server_path", cfg.EFlint.ServerPath),
	)

	// Initialize eFLINT Instance API handler
	instanceAPIHandler := eflint.NewInstanceAPIHandler(eflintManager, logger)

	// Initialize eFLINT State Manager (POC for export/import)
	stateManager := eflint.NewStateManager(eflintManager, "/tmp/eflint-states", logger)
	stateAPIHandler := eflint.NewStateAPIHandler(stateManager, logger)
	logger.Info("eFLINT state manager initialized (POC)")

	// Initialize RabbitMQ consumer
	// amqpURL := fmt.Sprintf("amqp://%s:%s@%s:%d/",
	// 	cfg.RabbitMQ.Username,
	// 	cfg.RabbitMQ.Password,
	// 	cfg.RabbitMQ.Host,
	// 	cfg.RabbitMQ.Port,
	// )

	// consumer, err := rabbitmq.NewConsumer(
	// 	amqpURL,
	// 	cfg.RabbitMQ.Queue,
	// 	cfg.RabbitMQ.PrefetchCount,
	// 	logger,
	// )
	// if err != nil {
	// 	logger.Fatal("failed to create RabbitMQ consumer", zap.Error(err))
	// }
	// defer consumer.Close()

	// // Initialize handler
	// reqHandler := handler.NewHandler(eflintClient, policyCache, logger)

	// Initialize HTTP server
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Define HTTP endpoints
	e.GET("/", func(c echo.Context) error {
		return c.HTML(http.StatusOK, "Hello, Policy Enforcer! <3")
	})

	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, struct{ Status string }{Status: "OK"})
	})

	// Register eFLINT Instance API routes
	eflintGroup := e.Group("/eflint")
	instanceAPIHandler.RegisterRoutes(eflintGroup)

	// Register eFLINT State Management API routes (POC)
	stateGroup := e.Group("/eflint/state")
	stateAPIHandler.RegisterRoutes(stateGroup)

	// Create the eFLINT reasoner (implements the Reasoner interface)
	eflintReasoner := reasoner.NewEflintReasoner(eflintManager, logger)

	// Create the policy enforcer (uses the Reasoner interface)
	enforcer := policyenforcer.NewEnforcer(eflintReasoner, logger)

	// Register HTTP handlers for policy enforcer
	policyEnforcerGroup := e.Group("/policy-enforcer")
	policyEnforcerHandler := policyenforcer.NewHTTPHandler(enforcer, logger)
	policyEnforcerHandler.RegisterRoutes(policyEnforcerGroup)

	// Auto-start eFLINT server with the configured model
	if cfg.EFlint.ModelPath != "" {
		logger.Info("auto-starting eFLINT server",
			zap.String("model", cfg.EFlint.ModelPath),
		)
		if err := eflintManager.Start(cfg.EFlint.ModelPath); err != nil {
			logger.Error("failed to auto-start eFLINT server", zap.Error(err))
			// Continue anyway - the server can be started manually via API
		}
	}

	// Get HTTP port from environment or use default
	httpPort := os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}

	// Start HTTP server in a goroutine
	go func() {
		logger.Info("starting HTTP server", zap.String("port", httpPort))
		if err := e.Start(":" + httpPort); err != nil && err != http.ErrServerClosed {
			logger.Fatal("failed to start HTTP server", zap.Error(err))
		}
	}()

	// Start consuming messages
	// msgs, err := consumer.Consume()
	// if err != nil {
	// 	logger.Fatal("failed to start consuming messages", zap.Error(err))
	// }

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	logger.Info("Policy Enforcer started, waiting for messages...")

	// Message processing loop
	// go func() {
	// 	for msg := range msgs {
	// 		if err := reqHandler.Handle(msg); err != nil {
	// 			logger.Error("failed to handle message", zap.Error(err))
	// 		}
	// 	}
	// }()

	// Wait for shutdown signal
	<-sigChan
	logger.Info("shutting down Policy Enforcer...")

	// Stop eFLINT instance if running
	if eflintManager.IsRunning() {
		logger.Info("stopping eFLINT instance...")
		if err := eflintManager.Stop(); err != nil {
			logger.Error("failed to stop eFLINT instance", zap.Error(err))
		}
	}

	// Gracefully shutdown HTTP server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		logger.Error("failed to shutdown HTTP server gracefully", zap.Error(err))
	}
}

// initLogger creates a configured zap logger
func initLogger(cfg config.LoggingConfig) *zap.Logger {
	// Parse log level
	level := zapcore.InfoLevel
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	// Create config
	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(level),
		Development:      cfg.Development,
		Encoding:         cfg.Format,
		EncoderConfig:    zap.NewProductionEncoderConfig(),
		OutputPaths:      []string{cfg.Output},
		ErrorOutputPaths: []string{"stderr"},
	}

	if cfg.Format == "console" {
		zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		zapConfig.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := zapConfig.Build()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	return logger
}
