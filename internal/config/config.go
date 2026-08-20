package config

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const maxSecretFileSize = 64 * 1024

type TradingMode string

const (
	ModeDisabled TradingMode = "disabled"
	ModeDryRun   TradingMode = "dry_run"
	ModeSandbox  TradingMode = "sandbox"
	ModeLive     TradingMode = "live"
)

type Config struct {
	Environment       string
	RequestedMode     TradingMode
	HTTPAddr          string
	MetricsAddr       string
	LogLevel          string
	ShutdownTimeout   time.Duration
	Database          DatabaseConfig
	RequiredMigration int64
}

// EffectiveMode is deliberately fixed until the control-plane, reconciliation,
// leader lock, and broker safety gates are implemented.
func (Config) EffectiveMode() TradingMode {
	return ModeDisabled
}

type DatabaseConfig struct {
	Host     string
	Port     uint16
	Name     string
	User     string
	password string
}

func (c DatabaseConfig) ConnectionString() string {
	dsn := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.password),
		Host:   net.JoinHostPort(c.Host, strconv.FormatUint(uint64(c.Port), 10)),
		Path:   c.Name,
	}
	query := dsn.Query()
	query.Set("sslmode", "disable")
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

type lookupEnv func(string) (string, bool)
type readFile func(string) ([]byte, error)

func Load(ctx context.Context) (Config, error) {
	return load(ctx, os.LookupEnv, os.ReadFile)
}

func load(ctx context.Context, lookup lookupEnv, read readFile) (Config, error) {
	if err := ctx.Err(); err != nil {
		return Config{}, err
	}

	requestedMode, err := parseMode(envOrDefault(lookup, "TRADING_MODE", string(ModeDisabled)))
	if err != nil {
		return Config{}, err
	}

	databasePort, err := parsePort(envOrDefault(lookup, "DATABASE_PORT", "5432"))
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := time.ParseDuration(envOrDefault(lookup, "SHUTDOWN_TIMEOUT", "10s"))
	if err != nil || shutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("parse SHUTDOWN_TIMEOUT: must be a positive duration")
	}

	passwordPath := envOrDefault(lookup, "DATABASE_PASSWORD_FILE", "/run/secrets/postgres_password")
	password, err := readSecret(ctx, read, passwordPath)
	if err != nil {
		return Config{}, fmt.Errorf("read database password file: %w", err)
	}

	cfg := Config{
		Environment:       envOrDefault(lookup, "APP_ENV", "development"),
		RequestedMode:     requestedMode,
		HTTPAddr:          envOrDefault(lookup, "HTTP_ADDR", "127.0.0.1:8080"),
		MetricsAddr:       envOrDefault(lookup, "METRICS_ADDR", "127.0.0.1:9090"),
		LogLevel:          envOrDefault(lookup, "LOG_LEVEL", "info"),
		ShutdownTimeout:   shutdownTimeout,
		RequiredMigration: 1,
		Database: DatabaseConfig{
			Host:     envOrDefault(lookup, "DATABASE_HOST", "postgres"),
			Port:     databasePort,
			Name:     envOrDefault(lookup, "DATABASE_NAME", "ladder_trader"),
			User:     envOrDefault(lookup, "DATABASE_USER", "ladder_trader"),
			password: password,
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var validationErrors []error
	if strings.TrimSpace(c.Environment) == "" {
		validationErrors = append(validationErrors, errors.New("APP_ENV must not be empty"))
	}
	if err := validateAddress("HTTP_ADDR", c.HTTPAddr); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if err := validateAddress("METRICS_ADDR", c.MetricsAddr); err != nil {
		validationErrors = append(validationErrors, err)
	}
	if c.HTTPAddr == c.MetricsAddr {
		validationErrors = append(validationErrors, errors.New("HTTP_ADDR and METRICS_ADDR must differ"))
	}
	if strings.TrimSpace(c.Database.Host) == "" {
		validationErrors = append(validationErrors, errors.New("DATABASE_HOST must not be empty"))
	}
	if strings.TrimSpace(c.Database.Name) == "" {
		validationErrors = append(validationErrors, errors.New("DATABASE_NAME must not be empty"))
	}
	if strings.TrimSpace(c.Database.User) == "" {
		validationErrors = append(validationErrors, errors.New("DATABASE_USER must not be empty"))
	}
	if c.Database.password == "" {
		validationErrors = append(validationErrors, errors.New("database password must not be empty"))
	}
	if c.RequiredMigration <= 0 {
		validationErrors = append(validationErrors, errors.New("required migration must be positive"))
	}
	return errors.Join(validationErrors...)
}

func parseMode(value string) (TradingMode, error) {
	mode := TradingMode(strings.TrimSpace(value))
	switch mode {
	case ModeDisabled, ModeDryRun, ModeSandbox, ModeLive:
		return mode, nil
	default:
		return "", fmt.Errorf("parse TRADING_MODE: unsupported value %q", value)
	}
}

func parsePort(value string) (uint16, error) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("parse DATABASE_PORT: must be between 1 and 65535")
	}
	return uint16(port), nil
}

func readSecret(ctx context.Context, read readFile, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("secret file path must not be empty")
	}
	data, err := read(path)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(data) > maxSecretFileSize {
		return "", errors.New("secret file exceeds size limit")
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", errors.New("secret file is empty")
	}
	return secret, nil
}

func validateAddress(name, address string) error {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return fmt.Errorf("parse %s: %w", name, err)
	}
	return nil
}

func envOrDefault(lookup lookupEnv, key, fallback string) string {
	if value, ok := lookup(key); ok {
		return value
	}
	return fallback
}
