package config

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLoadDefaultsToDisabled(t *testing.T) {
	t.Parallel()

	cfg, err := load(context.Background(), mapLookup(nil), fixedSecret("dev-password"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if got := cfg.EffectiveMode(); got != ModeDisabled {
		t.Fatalf("effective mode = %q, want %q", got, ModeDisabled)
	}
	if got := cfg.RequestedMode; got != ModeDisabled {
		t.Fatalf("requested mode = %q, want %q", got, ModeDisabled)
	}
}

func TestLiveRequestStillStartsDisabled(t *testing.T) {
	t.Parallel()

	cfg, err := load(context.Background(), mapLookup(map[string]string{
		"APP_ENV":      "production",
		"TRADING_MODE": "live",
	}), fixedSecret("prod-password"))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.RequestedMode != ModeLive {
		t.Fatalf("requested mode = %q, want %q", cfg.RequestedMode, ModeLive)
	}
	if cfg.EffectiveMode() != ModeDisabled {
		t.Fatalf("effective mode = %q, want %q", cfg.EffectiveMode(), ModeDisabled)
	}
}

func TestLoadRejectsUnsupportedMode(t *testing.T) {
	t.Parallel()

	_, err := load(context.Background(), mapLookup(map[string]string{
		"TRADING_MODE": "paper",
	}), fixedSecret("dev-password"))
	if err == nil || !strings.Contains(err.Error(), "unsupported value") {
		t.Fatalf("error = %v, want unsupported mode error", err)
	}
}

func TestLoadDoesNotExposeSecretInError(t *testing.T) {
	t.Parallel()

	secret := "do-not-log-this"
	_, err := load(context.Background(), mapLookup(map[string]string{
		"HTTP_ADDR": "invalid",
	}), fixedSecret(secret))
	if err == nil {
		t.Fatal("expected validation error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposes secret: %v", err)
	}
}

func TestLoadReadsDatabasePasswordFromEnvironment(t *testing.T) {
	t.Parallel()

	readCalled := false
	cfg, err := load(context.Background(), mapLookup(map[string]string{
		"DATABASE_PASSWORD": "env-password",
	}), func(string) ([]byte, error) {
		readCalled = true
		return nil, errors.New("must not read password file")
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if readCalled {
		t.Fatal("password file was read when DATABASE_PASSWORD is set")
	}
	if cfg.Database.password != "env-password" {
		t.Fatal("database password was not loaded from environment")
	}
}

func TestLoadRejectsEmptyDatabasePasswordEnvironment(t *testing.T) {
	t.Parallel()

	_, err := load(context.Background(), mapLookup(map[string]string{
		"DATABASE_PASSWORD": "   ",
	}), fixedSecret("file-password"))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_PASSWORD must not be empty") {
		t.Fatalf("error = %v, want empty DATABASE_PASSWORD error", err)
	}
}

func TestLoadPropagatesCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := load(ctx, mapLookup(nil), fixedSecret("dev-password"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func mapLookup(values map[string]string) lookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func fixedSecret(value string) readFile {
	return func(string) ([]byte, error) {
		return []byte(value), nil
	}
}
