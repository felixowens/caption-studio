// Package config provides the configuration for the application.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"main/pkg/logging"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the configuration for the application.
type Config struct {
	Service  ServiceConfig
	Database DatabaseConfig
	Server   ServerConfig
	Logging  logging.Config
}

// ServiceConfig is the configuration for the service.
type ServiceConfig struct {
	Name       string
	Version    string
	Identifier string
}

// DatabaseConfig is the configuration for the database.
type DatabaseConfig struct {
	File            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// ServerConfig is the configuration for the server.
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// getIdentifier generates a unique identifier for the service so that it can be used to identify the service in the logs.
func getIdentifier() string {
	hostname, err := os.Hostname()
	if hostname == "" || err != nil {
		hostname = "localhost"
	}
	var buf [12]byte
	var b64 string
	for len(b64) < 10 {
		rand.Read(buf[:])
		b64 = base64.StdEncoding.EncodeToString(buf[:])
		b64 = strings.NewReplacer("+", "", "/", "").Replace(b64)
	}

	return fmt.Sprintf("%s/%s", hostname, b64[0:10])
}

// Load loads the configuration from the environment variables.
func Load() (*Config, error) {

	config := &Config{
		Service: ServiceConfig{
			Name:       getEnv("SERVICE_NAME", "Caption Studio API"),
			Version:    getEnv("SERVICE_VERSION", "0.1.0"),
			Identifier: getIdentifier(),
		},
		Database: DatabaseConfig{
			File:            getEnv("APP_DB_FILE", "data/app.db"),
			MaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		Server: ServerConfig{
			Port:            getEnv("PORT", "8888"),
			ReadTimeout:     getEnvDuration("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getEnvDuration("SERVER_WRITE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Logging: logging.Config{
			Level:  logging.LogLevel(getEnv("LOG_LEVEL", "info")),
			Format: logging.LogFormat(getEnv("LOG_FORMAT", "text")),
		},
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return config, nil
}

func (c *Config) validate() error {
	if c.Database.File == "" {
		return fmt.Errorf("APP_DB_FILE is required")
	}
	if c.Server.Port == "" {
		return fmt.Errorf("PORT is required")
	}
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("DB_MAX_OPEN_CONNS must be positive")
	}
	if c.Database.MaxIdleConns <= 0 {
		return fmt.Errorf("DB_MAX_IDLE_CONNS must be positive")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

// getEnvBool is a helper function to get a boolean environment variable.
func _(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
