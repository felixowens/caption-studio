// Package main is the entry point for the server application.
package main

import (
	"fmt"
	"main/internal/config"
	"main/internal/db"
	"main/internal/service"
	"main/internal/transport/httpresource"
	pkghuma "main/pkg/huma"
	"main/pkg/logging"

	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	logger := logging.Setup(logging.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
	})
	logger.Debug("Arca server starting", "version", cfg.Service.Version)

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	server, err := setupServer(cfg, logger)
	if err != nil {
		logger.Error("Failed to setup server", "error", err)
		os.Exit(1)
	}

	go func() {
		logger.Info("Server started", "port", cfg.Server.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	cancel()
	logger.Info("Server shutdown complete")
}

func initializeService(cfg *config.Config, logger *slog.Logger) (*service.Service, error) {
	database, err := db.InitDatabase(cfg.Database.File, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	database.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	database.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	database.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	queries := db.New(database)
	return service.NewService(queries, logger), nil
}

func setupServer(cfg *config.Config, logger *slog.Logger) (*http.Server, error) {
	s, err := initializeService(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize service: %w", err)
	}

	router := chi.NewMux()
	api := humachi.New(router, huma.DefaultConfig(cfg.Service.Name, cfg.Service.Version))

	// Register middleware
	api.UseMiddleware(pkghuma.NewLoggerMiddleware(logger))
	api.UseMiddleware(pkghuma.NewRequestIDMiddleware(cfg.Service.Identifier))
	api.UseMiddleware(pkghuma.GetRealIPMiddleware)

	server := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	httpresource.ProvisionCaptionsResource(api, *cfg, s.CaptionService, logger.With("component", "captions"))

	return server, nil
}
