package operations

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func Handler(state *State) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, _ *http.Request) {
		snapshot := state.Snapshot()
		status := http.StatusOK
		if !snapshot.OperationalReady {
			status = http.StatusServiceUnavailable
		}
		writeJSON(writer, status, snapshot)
	})
	mux.HandleFunc("GET /v1/status", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, state.Snapshot())
	})
	return securityHeaders(mux)
}

func MetricsHandler(state *State) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/metrics" {
			http.NotFound(writer, request)
			return
		}
		snapshot := state.Snapshot()
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		writer.Header().Set("Cache-Control", "no-store")
		_, _ = fmt.Fprintf(writer,
			"# TYPE trader_operational_ready gauge\ntrader_operational_ready %d\n"+
				"# TYPE trader_trading_ready gauge\ntrader_trading_ready %d\n"+
				"# TYPE trader_mode_info gauge\ntrader_mode_info{mode=%q} 1\n",
			boolValue(snapshot.OperationalReady),
			boolValue(snapshot.TradingReady),
			snapshot.EffectiveMode,
		)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func boolValue(value bool) int {
	if value {
		return 1
	}
	return 0
}
