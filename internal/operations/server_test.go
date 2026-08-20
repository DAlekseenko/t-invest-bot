package operations

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"t-invest-bot/internal/config"
)

func TestHealthIsAlwaysLive(t *testing.T) {
	t.Parallel()

	state := NewState(config.ModeLive, config.ModeDisabled, time.Unix(0, 0))
	response := httptest.NewRecorder()
	Handler(state).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestReadinessRequiresOperationalChecks(t *testing.T) {
	t.Parallel()

	state := NewState(config.ModeLive, config.ModeDisabled, time.Unix(0, 0))
	handler := Handler(state)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("initial status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}

	state.SetConfigValid(true)
	state.SetDatabase(true)
	state.SetMigrations(true)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("ready status = %d, want %d", response.Code, http.StatusOK)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshot); err != nil {
		t.Fatalf("decode readiness response: %v", err)
	}
	if snapshot.TradingReady {
		t.Fatal("trading must remain not ready while effective mode is disabled")
	}
	if snapshot.EffectiveMode != config.ModeDisabled {
		t.Fatalf("effective mode = %q, want disabled", snapshot.EffectiveMode)
	}
}

func TestMetricsReportDisabledTrading(t *testing.T) {
	t.Parallel()

	state := NewState(config.ModeSandbox, config.ModeDisabled, time.Unix(0, 0))
	state.SetConfigValid(true)
	state.SetDatabase(true)
	state.SetMigrations(true)

	response := httptest.NewRecorder()
	MetricsHandler(state).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, "trader_operational_ready 1") {
		t.Fatalf("metrics do not report operational readiness:\n%s", body)
	}
	if !strings.Contains(body, "trader_trading_ready 0") {
		t.Fatalf("metrics do not keep trading disabled:\n%s", body)
	}
}
