package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

var logger *slog.Logger

func initLogger() error {
	logLevel := getLogLevel()
	
	// Create log directory
	logDir := filepath.Join("data", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %v", err)
	}

	// Create/truncate log file (wipe on restart)
	logFile := filepath.Join(logDir, "app.log")
	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	// Create multi-writer for console and file (unused but shows intent)
	_ = io.MultiWriter(os.Stdout, file)

	// Console handler (pretty printed)
	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Make timestamps more readable in console
			if a.Key == slog.TimeKey {
				return slog.Attr{
					Key:   a.Key,
					Value: slog.StringValue(a.Value.Time().Format("15:04:05")),
				}
			}
			return a
		},
	})

	// File handler (JSON format)
	fileHandler := slog.NewJSONHandler(file, &slog.HandlerOptions{
		Level: logLevel,
	})

	// Create a custom handler that writes to both outputs
	handler := &multiHandler{
		console: consoleHandler,
		file:    fileHandler,
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("Logger initialized", 
		"level", logLevel.String(),
		"file", logFile,
	)

	return nil
}

func getLogLevel() slog.Level {
	levelStr := strings.ToUpper(os.Getenv("LOG_LEVEL"))
	switch levelStr {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO", "":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// multiHandler implements slog.Handler to write to multiple outputs
type multiHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.console.Enabled(ctx, level) || h.file.Enabled(ctx, level)
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	// Write to console with pretty format
	if h.console.Enabled(ctx, record.Level) {
		if err := h.console.Handle(ctx, record); err != nil {
			return err
		}
	}
	
	// Write to file with JSON format
	if h.file.Enabled(ctx, record.Level) {
		if err := h.file.Handle(ctx, record); err != nil {
			return err
		}
	}
	
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{
		console: h.console.WithAttrs(attrs),
		file:    h.file.WithAttrs(attrs),
	}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{
		console: h.console.WithGroup(name),
		file:    h.file.WithGroup(name),
	}
}

// HTTP request logging middleware
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(data)
	rw.size += size
	return size, err
}

func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate request ID
		requestID := uuid.New().String()
		
		// Add request ID to context
		ctx := context.WithValue(r.Context(), "requestID", requestID)
		r = r.WithContext(ctx)
		
		// Wrap response writer
		rw := &responseWriter{
			ResponseWriter: w,
			statusCode:     200,
		}
		
		start := time.Now()
		
		// Add request ID header
		w.Header().Set("X-Request-ID", requestID)
		
		// Log request start
		logger.Debug("Request started",
			"method", r.Method,
			"path", r.URL.Path,
			"remote_addr", r.RemoteAddr,
			"user_agent", r.UserAgent(),
			"request_id", requestID,
		)
		
		// Process request
		next.ServeHTTP(rw, r)
		
		duration := time.Since(start)
		
		// Log request completion
		logLevel := slog.LevelInfo
		if rw.statusCode >= 400 {
			logLevel = slog.LevelWarn
		}
		if rw.statusCode >= 500 {
			logLevel = slog.LevelError
		}
		
		logger.Log(r.Context(), logLevel, "Request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.statusCode,
			"duration_ms", duration.Milliseconds(),
			"size_bytes", rw.size,
			"request_id", requestID,
		)
	})
}

// Helper function to get request ID from context
func getRequestID(ctx context.Context) string {
	if id, ok := ctx.Value("requestID").(string); ok {
		return id
	}
	return ""
}

// Helper functions for common log patterns
func logError(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	allAttrs := append(attrs, slog.String("error", err.Error()))
	if requestID := getRequestID(ctx); requestID != "" {
		allAttrs = append(allAttrs, slog.String("request_id", requestID))
	}
	logger.LogAttrs(ctx, slog.LevelError, msg, allAttrs...)
}

func logInfo(ctx context.Context, msg string, attrs ...slog.Attr) {
	if requestID := getRequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

func logDebug(ctx context.Context, msg string, attrs ...slog.Attr) {
	if requestID := getRequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

func logWarn(ctx context.Context, msg string, attrs ...slog.Attr) {
	if requestID := getRequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
}