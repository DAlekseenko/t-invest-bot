package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

type SellScope string

const (
	SellScopeLadderOnly     SellScope = "ladder_only"
	SellScopeEntirePosition SellScope = "entire_position"
)

type Control struct {
	SchemaVersion        int64
	PublishedRevision    int64
	Approved             bool
	Mode                 TradingMode
	PauseNewOrders       bool
	AccountAlias         string
	MaxOrderNano         Nano
	MaxTotalReserveNano  Nano
	MaxDailyTurnoverNano Nano
	MaxOpenOrders        int64
	ConfigValidUntil     time.Time
	Comment              string
}

type LadderLevel struct {
	Revision                 int64
	StrategyID               string
	Enabled                  bool
	InstrumentUID            string
	Ticker                   string
	LevelNo                  int64
	BasePositionQty          int64
	BasePositionAvgPriceNano Nano
	SellScope                SellScope
	StepBudgetNano           Nano
	EntryCorrectionNano      Nano
	ExitCorrectionNano       Nano
	PreviewBuyPriceNano      Nano
	PreviewBuyLots           int64
	PreviewSellPriceNano     Nano
	PreviewSellQty           int64
	MaxLevelAmountNano       Nano
	AutoRestart              bool
	Comment                  string
}

type Snapshot struct {
	control Control
	levels  []LadderLevel
	hash    string
}

func NewSnapshot(control Control, levels []LadderLevel) (Snapshot, error) {
	clonedLevels := slices.Clone(levels)
	slices.SortFunc(clonedLevels, func(left, right LadderLevel) int {
		if left.StrategyID < right.StrategyID {
			return -1
		}
		if left.StrategyID > right.StrategyID {
			return 1
		}
		if left.LevelNo < right.LevelNo {
			return -1
		}
		if left.LevelNo > right.LevelNo {
			return 1
		}
		return 0
	})

	canonical := struct {
		Control canonicalControl `json:"control"`
		Levels  []LadderLevel    `json:"levels"`
	}{
		Control: canonicalizeControl(control),
		Levels:  clonedLevels,
	}
	contents, err := json.Marshal(canonical)
	if err != nil {
		return Snapshot{}, fmt.Errorf("marshal canonical snapshot: %w", err)
	}
	sum := sha256.Sum256(contents)
	return Snapshot{
		control: control,
		levels:  clonedLevels,
		hash:    hex.EncodeToString(sum[:]),
	}, nil
}

func (snapshot Snapshot) Control() Control {
	return snapshot.control
}

func (snapshot Snapshot) Levels() []LadderLevel {
	return slices.Clone(snapshot.levels)
}

func (snapshot Snapshot) Hash() string {
	return snapshot.hash
}

type canonicalControl struct {
	SchemaVersion        int64       `json:"schema_version"`
	PublishedRevision    int64       `json:"published_revision"`
	Approved             bool        `json:"approved"`
	Mode                 TradingMode `json:"mode"`
	PauseNewOrders       bool        `json:"pause_new_orders"`
	AccountAlias         string      `json:"account_alias"`
	MaxOrderNano         Nano        `json:"max_order_nano"`
	MaxTotalReserveNano  Nano        `json:"max_total_reserve_nano"`
	MaxDailyTurnoverNano Nano        `json:"max_daily_turnover_nano"`
	MaxOpenOrders        int64       `json:"max_open_orders"`
	ConfigValidUntil     string      `json:"config_valid_until"`
	Comment              string      `json:"comment"`
}

func canonicalizeControl(control Control) canonicalControl {
	return canonicalControl{
		SchemaVersion:        control.SchemaVersion,
		PublishedRevision:    control.PublishedRevision,
		Approved:             control.Approved,
		Mode:                 control.Mode,
		PauseNewOrders:       control.PauseNewOrders,
		AccountAlias:         control.AccountAlias,
		MaxOrderNano:         control.MaxOrderNano,
		MaxTotalReserveNano:  control.MaxTotalReserveNano,
		MaxDailyTurnoverNano: control.MaxDailyTurnoverNano,
		MaxOpenOrders:        control.MaxOpenOrders,
		ConfigValidUntil:     control.ConfigValidUntil.UTC().Format(time.RFC3339Nano),
		Comment:              control.Comment,
	}
}
