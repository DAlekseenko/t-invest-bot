package operations

import (
	"sync"
	"time"

	"t-invest-bot/internal/config"
)

type State struct {
	mu            sync.RWMutex
	startedAt     time.Time
	requestedMode config.TradingMode
	effectiveMode config.TradingMode
	checks        Checks
}

type Checks struct {
	ConfigValid    bool `json:"config_valid"`
	Database       bool `json:"database"`
	Migrations     bool `json:"migrations"`
	Broker         bool `json:"broker"`
	Reconciliation bool `json:"reconciliation"`
	LeaderLock     bool `json:"leader_lock"`
}

type Snapshot struct {
	StartedAt        time.Time          `json:"started_at"`
	RequestedMode    config.TradingMode `json:"requested_mode"`
	EffectiveMode    config.TradingMode `json:"effective_mode"`
	OperationalReady bool               `json:"operational_ready"`
	TradingReady     bool               `json:"trading_ready"`
	Checks           Checks             `json:"checks"`
}

func NewState(requestedMode, effectiveMode config.TradingMode, startedAt time.Time) *State {
	return &State{
		startedAt:     startedAt.UTC(),
		requestedMode: requestedMode,
		effectiveMode: effectiveMode,
	}
}

func (s *State) SetConfigValid(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks.ConfigValid = value
}

func (s *State) SetDatabase(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks.Database = value
}

func (s *State) SetMigrations(value bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checks.Migrations = value
}

func (s *State) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	operationalReady := s.checks.ConfigValid && s.checks.Database && s.checks.Migrations
	tradingReady := operationalReady &&
		s.effectiveMode != config.ModeDisabled &&
		s.checks.Broker &&
		s.checks.Reconciliation &&
		s.checks.LeaderLock

	return Snapshot{
		StartedAt:        s.startedAt,
		RequestedMode:    s.requestedMode,
		EffectiveMode:    s.effectiveMode,
		OperationalReady: operationalReady,
		TradingReady:     tradingReady,
		Checks:           s.checks,
	}
}
