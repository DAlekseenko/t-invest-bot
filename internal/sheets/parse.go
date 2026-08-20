package sheets

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"t-invest-bot/internal/config"
)

const (
	controlSheet = "BOT_CONTROL"
	configSheet  = "BOT_CONFIG"
)

type ContractError struct {
	Sheet string
	Row   int
	Field string
	Code  string
	Err   error
}

func (err *ContractError) Error() string {
	location := err.Sheet
	if err.Row > 0 {
		location += fmt.Sprintf(" row %d", err.Row)
	}
	if err.Field != "" {
		location += " field " + err.Field
	}
	if err.Err != nil {
		return fmt.Sprintf("%s: %s: %v", location, err.Code, err.Err)
	}
	return fmt.Sprintf("%s: %s", location, err.Code)
}

func (err *ContractError) Unwrap() error {
	return err.Err
}

func parseControl(rows [][]string, now time.Time) (config.Control, error) {
	values := make(map[string]string)
	rowByKey := make(map[string]int)
	for index, row := range rows {
		rowNumber := index + 1
		if emptyRow(row) {
			continue
		}
		key := strings.TrimSpace(cell(row, 0))
		value := strings.TrimSpace(cell(row, 1))
		if rowNumber == 1 && strings.EqualFold(key, "key") && strings.EqualFold(value, "value") {
			continue
		}
		if key == "" {
			return config.Control{}, contractError(controlSheet, rowNumber, "key", "EMPTY_KEY", nil)
		}
		if _, known := controlFields[key]; !known {
			return config.Control{}, contractError(controlSheet, rowNumber, key, "UNKNOWN_FIELD", nil)
		}
		if _, duplicate := values[key]; duplicate {
			return config.Control{}, contractError(controlSheet, rowNumber, key, "DUPLICATE_FIELD", nil)
		}
		values[key] = value
		rowByKey[key] = rowNumber
	}

	for field := range requiredControlFields {
		if _, ok := values[field]; !ok {
			return config.Control{}, contractError(controlSheet, 0, field, "MISSING_FIELD", nil)
		}
	}

	parseIntegerField := func(field string) (int64, error) {
		value, err := parseInteger(values[field])
		if err != nil {
			return 0, contractError(controlSheet, rowByKey[field], field, "INVALID_INTEGER", err)
		}
		return value, nil
	}
	parseNanoField := func(field string) (config.Nano, error) {
		value, err := config.ParseNano(values[field])
		if err != nil {
			return 0, contractError(controlSheet, rowByKey[field], field, "INVALID_DECIMAL", err)
		}
		return value, nil
	}
	parseBooleanField := func(field string) (bool, error) {
		value, err := parseBoolean(values[field])
		if err != nil {
			return false, contractError(controlSheet, rowByKey[field], field, "INVALID_BOOLEAN", err)
		}
		return value, nil
	}

	schemaVersion, err := parseIntegerField("schema_version")
	if err != nil {
		return config.Control{}, err
	}
	revision, err := parseIntegerField("published_revision")
	if err != nil {
		return config.Control{}, err
	}
	approved, err := parseBooleanField("approved")
	if err != nil {
		return config.Control{}, err
	}
	mode, err := config.ParseTradingMode(values["mode"])
	if err != nil {
		return config.Control{}, contractError(controlSheet, rowByKey["mode"], "mode", "INVALID_MODE", err)
	}
	pauseNewOrders, err := parseBooleanField("pause_new_orders")
	if err != nil {
		return config.Control{}, err
	}
	maxOrder, err := parseNanoField("max_order_rub")
	if err != nil {
		return config.Control{}, err
	}
	maxTotalReserve, err := parseNanoField("max_total_reserve_rub")
	if err != nil {
		return config.Control{}, err
	}
	maxDailyTurnover, err := parseNanoField("max_daily_turnover_rub")
	if err != nil {
		return config.Control{}, err
	}
	maxOpenOrders, err := parseIntegerField("max_open_orders")
	if err != nil {
		return config.Control{}, err
	}
	validUntil, err := time.Parse(time.RFC3339, values["config_valid_until"])
	if err != nil {
		return config.Control{}, contractError(
			controlSheet,
			rowByKey["config_valid_until"],
			"config_valid_until",
			"INVALID_TIMESTAMP",
			err,
		)
	}

	control := config.Control{
		SchemaVersion:        schemaVersion,
		PublishedRevision:    revision,
		Approved:             approved,
		Mode:                 mode,
		PauseNewOrders:       pauseNewOrders,
		AccountAlias:         strings.TrimSpace(values["account_alias"]),
		MaxOrderNano:         maxOrder,
		MaxTotalReserveNano:  maxTotalReserve,
		MaxDailyTurnoverNano: maxDailyTurnover,
		MaxOpenOrders:        maxOpenOrders,
		ConfigValidUntil:     validUntil.UTC(),
		Comment:              strings.TrimSpace(values["comment"]),
	}
	if err := validateControl(control, now); err != nil {
		return config.Control{}, err
	}
	return control, nil
}

func parseLevels(rows [][]string, publishedRevision int64) ([]config.LadderLevel, error) {
	headerIndex := -1
	for index, row := range rows {
		if !emptyRow(row) {
			headerIndex = index
			break
		}
	}
	if headerIndex < 0 {
		return nil, contractError(configSheet, 0, "", "EMPTY_SHEET", nil)
	}
	header, err := parseHeader(rows[headerIndex], headerIndex+1)
	if err != nil {
		return nil, err
	}

	levels := make([]config.LadderLevel, 0, len(rows)-headerIndex-1)
	for index := headerIndex + 1; index < len(rows); index++ {
		if emptyRow(rows[index]) {
			continue
		}
		level, err := parseLevel(rows[index], index+1, header, publishedRevision)
		if err != nil {
			return nil, err
		}
		levels = append(levels, level)
	}
	if len(levels) == 0 {
		return nil, contractError(configSheet, 0, "", "NO_LEVELS", nil)
	}
	if err := validateLevels(levels); err != nil {
		return nil, err
	}
	return levels, nil
}

func parseHeader(row []string, rowNumber int) (map[string]int, error) {
	header := make(map[string]int)
	for index, rawName := range row {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		if _, known := levelFields[name]; !known {
			return nil, contractError(configSheet, rowNumber, name, "UNKNOWN_FIELD", nil)
		}
		if _, duplicate := header[name]; duplicate {
			return nil, contractError(configSheet, rowNumber, name, "DUPLICATE_FIELD", nil)
		}
		header[name] = index
	}
	for field := range requiredLevelFields {
		if _, ok := header[field]; !ok {
			return nil, contractError(configSheet, rowNumber, field, "MISSING_FIELD", nil)
		}
	}
	return header, nil
}

func parseLevel(row []string, rowNumber int, header map[string]int, publishedRevision int64) (config.LadderLevel, error) {
	value := func(field string) string {
		return strings.TrimSpace(cell(row, header[field]))
	}
	integer := func(field string) (int64, error) {
		parsed, err := parseInteger(value(field))
		if err != nil {
			return 0, contractError(configSheet, rowNumber, field, "INVALID_INTEGER", err)
		}
		return parsed, nil
	}
	decimal := func(field string) (config.Nano, error) {
		parsed, err := config.ParseNano(value(field))
		if err != nil {
			return 0, contractError(configSheet, rowNumber, field, "INVALID_DECIMAL", err)
		}
		return parsed, nil
	}
	boolean := func(field string) (bool, error) {
		parsed, err := parseBoolean(value(field))
		if err != nil {
			return false, contractError(configSheet, rowNumber, field, "INVALID_BOOLEAN", err)
		}
		return parsed, nil
	}

	revision, err := integer("revision")
	if err != nil {
		return config.LadderLevel{}, err
	}
	if revision != publishedRevision {
		return config.LadderLevel{}, contractError(configSheet, rowNumber, "revision", "REVISION_MISMATCH", nil)
	}
	enabled, err := boolean("enabled")
	if err != nil {
		return config.LadderLevel{}, err
	}
	levelNumber, err := integer("level_no")
	if err != nil {
		return config.LadderLevel{}, err
	}
	basePositionQty, err := integer("base_position_qty")
	if err != nil {
		return config.LadderLevel{}, err
	}
	basePositionAverage, err := decimal("base_position_avg_price")
	if err != nil {
		return config.LadderLevel{}, err
	}
	sellScope := config.SellScope(value("sell_scope"))
	if sellScope != config.SellScopeLadderOnly && sellScope != config.SellScopeEntirePosition {
		return config.LadderLevel{}, contractError(configSheet, rowNumber, "sell_scope", "INVALID_SELL_SCOPE", nil)
	}
	stepBudget, err := decimal("step_budget_rub")
	if err != nil {
		return config.LadderLevel{}, err
	}
	entryCorrection, err := decimal("entry_correction")
	if err != nil {
		return config.LadderLevel{}, err
	}
	exitCorrection, err := decimal("exit_correction")
	if err != nil {
		return config.LadderLevel{}, err
	}
	previewBuyPrice, err := decimal("preview_buy_price")
	if err != nil {
		return config.LadderLevel{}, err
	}
	previewBuyLots, err := integer("preview_buy_lots")
	if err != nil {
		return config.LadderLevel{}, err
	}
	previewSellPrice, err := decimal("preview_sell_price")
	if err != nil {
		return config.LadderLevel{}, err
	}
	previewSellQty, err := integer("preview_sell_qty")
	if err != nil {
		return config.LadderLevel{}, err
	}
	maxLevelAmount, err := decimal("max_level_amount_rub")
	if err != nil {
		return config.LadderLevel{}, err
	}
	autoRestart, err := boolean("auto_restart")
	if err != nil {
		return config.LadderLevel{}, err
	}

	level := config.LadderLevel{
		Revision:                 revision,
		StrategyID:               strings.TrimSpace(value("strategy_id")),
		Enabled:                  enabled,
		InstrumentUID:            strings.TrimSpace(value("instrument_uid")),
		Ticker:                   strings.ToUpper(value("ticker")),
		LevelNo:                  levelNumber,
		BasePositionQty:          basePositionQty,
		BasePositionAvgPriceNano: basePositionAverage,
		SellScope:                sellScope,
		StepBudgetNano:           stepBudget,
		EntryCorrectionNano:      entryCorrection,
		ExitCorrectionNano:       exitCorrection,
		PreviewBuyPriceNano:      previewBuyPrice,
		PreviewBuyLots:           previewBuyLots,
		PreviewSellPriceNano:     previewSellPrice,
		PreviewSellQty:           previewSellQty,
		MaxLevelAmountNano:       maxLevelAmount,
		AutoRestart:              autoRestart,
		Comment:                  strings.TrimSpace(value("comment")),
	}
	if err := validateLevel(level, rowNumber); err != nil {
		return config.LadderLevel{}, err
	}
	return level, nil
}

func validateControl(control config.Control, now time.Time) error {
	switch {
	case control.SchemaVersion != 1:
		return contractError(controlSheet, 0, "schema_version", "UNSUPPORTED_SCHEMA", nil)
	case control.PublishedRevision <= 0:
		return contractError(controlSheet, 0, "published_revision", "OUT_OF_RANGE", nil)
	case !control.Approved:
		return contractError(controlSheet, 0, "approved", "NOT_APPROVED", nil)
	case control.AccountAlias == "":
		return contractError(controlSheet, 0, "account_alias", "EMPTY_VALUE", nil)
	case control.MaxOrderNano <= 0:
		return contractError(controlSheet, 0, "max_order_rub", "OUT_OF_RANGE", nil)
	case control.MaxTotalReserveNano <= 0:
		return contractError(controlSheet, 0, "max_total_reserve_rub", "OUT_OF_RANGE", nil)
	case control.MaxDailyTurnoverNano <= 0:
		return contractError(controlSheet, 0, "max_daily_turnover_rub", "OUT_OF_RANGE", nil)
	case control.MaxOpenOrders <= 0:
		return contractError(controlSheet, 0, "max_open_orders", "OUT_OF_RANGE", nil)
	case control.Mode == config.ModeLive && !now.Before(control.ConfigValidUntil):
		return contractError(controlSheet, 0, "config_valid_until", "EXPIRED_LIVE_CONFIG", nil)
	default:
		return nil
	}
}

func validateLevel(level config.LadderLevel, rowNumber int) error {
	switch {
	case level.StrategyID == "":
		return contractError(configSheet, rowNumber, "strategy_id", "EMPTY_VALUE", nil)
	case level.InstrumentUID == "":
		return contractError(configSheet, rowNumber, "instrument_uid", "EMPTY_VALUE", nil)
	case level.Ticker == "":
		return contractError(configSheet, rowNumber, "ticker", "EMPTY_VALUE", nil)
	case level.LevelNo <= 0:
		return contractError(configSheet, rowNumber, "level_no", "OUT_OF_RANGE", nil)
	case level.BasePositionQty < 0:
		return contractError(configSheet, rowNumber, "base_position_qty", "OUT_OF_RANGE", nil)
	case level.BasePositionAvgPriceNano < 0:
		return contractError(configSheet, rowNumber, "base_position_avg_price", "OUT_OF_RANGE", nil)
	case level.SellScope == config.SellScopeEntirePosition:
		return contractError(configSheet, rowNumber, "sell_scope", "ENTIRE_POSITION_FORBIDDEN", nil)
	case level.StepBudgetNano <= 0:
		return contractError(configSheet, rowNumber, "step_budget_rub", "OUT_OF_RANGE", nil)
	case level.PreviewBuyPriceNano <= 0:
		return contractError(configSheet, rowNumber, "preview_buy_price", "OUT_OF_RANGE", nil)
	case level.PreviewBuyLots <= 0:
		return contractError(configSheet, rowNumber, "preview_buy_lots", "OUT_OF_RANGE", nil)
	case level.PreviewSellPriceNano <= 0:
		return contractError(configSheet, rowNumber, "preview_sell_price", "OUT_OF_RANGE", nil)
	case level.PreviewSellQty < 0:
		return contractError(configSheet, rowNumber, "preview_sell_qty", "OUT_OF_RANGE", nil)
	case level.MaxLevelAmountNano <= 0:
		return contractError(configSheet, rowNumber, "max_level_amount_rub", "OUT_OF_RANGE", nil)
	case level.AutoRestart:
		return contractError(configSheet, rowNumber, "auto_restart", "AUTO_RESTART_FORBIDDEN", nil)
	default:
		return nil
	}
}

func validateLevels(levels []config.LadderLevel) error {
	ordered := slices.Clone(levels)
	slices.SortFunc(ordered, func(left, right config.LadderLevel) int {
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

	seen := make(map[string]struct{}, len(ordered))
	for index, level := range ordered {
		key := fmt.Sprintf("%d/%s/%d", level.Revision, level.StrategyID, level.LevelNo)
		if _, duplicate := seen[key]; duplicate {
			return contractError(configSheet, 0, "level_no", "DUPLICATE_LEVEL", nil)
		}
		seen[key] = struct{}{}

		if index == 0 || ordered[index-1].StrategyID != level.StrategyID {
			if level.LevelNo != 1 {
				return contractError(configSheet, 0, "level_no", "LEVEL_SEQUENCE_GAP", nil)
			}
			continue
		}
		previous := ordered[index-1]
		if level.LevelNo != previous.LevelNo+1 {
			return contractError(configSheet, 0, "level_no", "LEVEL_SEQUENCE_GAP", nil)
		}
		if level.InstrumentUID != previous.InstrumentUID {
			return contractError(configSheet, 0, "instrument_uid", "STRATEGY_INSTRUMENT_CHANGED", nil)
		}
		if level.Ticker != previous.Ticker ||
			level.Enabled != previous.Enabled ||
			level.BasePositionQty != previous.BasePositionQty ||
			level.BasePositionAvgPriceNano != previous.BasePositionAvgPriceNano ||
			level.SellScope != previous.SellScope {
			return contractError(configSheet, 0, "strategy_id", "INCONSISTENT_STRATEGY_FIELDS", nil)
		}
		if level.PreviewBuyPriceNano >= previous.PreviewBuyPriceNano {
			return contractError(configSheet, 0, "preview_buy_price", "BUY_PRICES_NOT_DESCENDING", nil)
		}
	}
	return nil
}

func parseBoolean(value string) (bool, error) {
	switch value {
	case "TRUE":
		return true, nil
	case "FALSE":
		return false, nil
	default:
		return false, errors.New("expected TRUE or FALSE")
	}
}

func parseInteger(value string) (int64, error) {
	normalized := strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(strings.TrimSpace(value))
	if normalized == "" {
		return 0, errors.New("integer is empty")
	}
	parsed, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed, nil
}

func emptyRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func cell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func contractError(sheet string, row int, field, code string, err error) error {
	return &ContractError{Sheet: sheet, Row: row, Field: field, Code: code, Err: err}
}

var controlFields = map[string]struct{}{
	"schema_version": {}, "published_revision": {}, "approved": {}, "mode": {},
	"pause_new_orders": {}, "account_alias": {}, "max_order_rub": {},
	"max_total_reserve_rub": {}, "max_daily_turnover_rub": {}, "max_open_orders": {},
	"config_valid_until": {}, "comment": {},
}

var requiredControlFields = map[string]struct{}{
	"schema_version": {}, "published_revision": {}, "approved": {}, "mode": {},
	"pause_new_orders": {}, "account_alias": {}, "max_order_rub": {},
	"max_total_reserve_rub": {}, "max_daily_turnover_rub": {}, "max_open_orders": {},
	"config_valid_until": {},
}

var levelFields = map[string]struct{}{
	"revision": {}, "strategy_id": {}, "enabled": {}, "instrument_uid": {}, "ticker": {},
	"level_no": {}, "base_position_qty": {}, "base_position_avg_price": {}, "sell_scope": {},
	"step_budget_rub": {}, "entry_correction": {}, "exit_correction": {},
	"preview_buy_price": {}, "preview_buy_lots": {}, "preview_sell_price": {},
	"preview_sell_qty": {}, "max_level_amount_rub": {}, "auto_restart": {}, "comment": {},
}

var requiredLevelFields = map[string]struct{}{
	"revision": {}, "strategy_id": {}, "enabled": {}, "instrument_uid": {}, "ticker": {},
	"level_no": {}, "base_position_qty": {}, "base_position_avg_price": {}, "sell_scope": {},
	"step_budget_rub": {}, "entry_correction": {}, "exit_correction": {},
	"preview_buy_price": {}, "preview_buy_lots": {}, "preview_sell_price": {},
	"preview_sell_qty": {}, "max_level_amount_rub": {}, "auto_restart": {},
}
