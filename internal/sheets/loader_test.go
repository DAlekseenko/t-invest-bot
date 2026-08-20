package sheets

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"t-invest-bot/internal/config"
)

func TestPublishedSnapshotMatchesGoldenFixture(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	configRows := readCSV(t, "testdata/BOT_CONFIG.csv")
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows, controlRows},
		ConfigRange:  {configRows, configRows},
	}})

	snapshot, err := loader.PublishedSnapshot(context.Background())
	if err != nil {
		t.Fatalf("load published snapshot: %v", err)
	}
	if snapshot.Control().PublishedRevision != 12 {
		t.Fatalf("revision = %d, want 12", snapshot.Control().PublishedRevision)
	}
	if len(snapshot.Hash()) != 64 {
		t.Fatalf("hash length = %d, want 64", len(snapshot.Hash()))
	}

	expectedPrices := map[string][]config.Nano{
		"SBER":  {275_480_000_000, 261_710_000_000, 246_550_000_000, 228_200_000_000, 203_770_000_000, 168_000_000_000},
		"TRNFP": {1_028_400_000_000, 976_980_000_000, 920_410_000_000, 851_960_000_000, 760_850_000_000, 627_450_000_000},
		"BTBR":  {114_460_000_000, 108_740_000_000, 102_440_000_000, 94_810_000_000, 84_650_000_000, 81_000_000_000},
	}
	expectedLots := map[string][]int64{
		"SBER":  {21, 27, 36, 47, 58, 89},
		"TRNFP": {4, 5, 6, 8, 10, 14},
		"BTBR":  {34, 45, 68, 73, 94, 111},
	}
	actualPrices := make(map[string][]config.Nano)
	actualLots := make(map[string][]int64)
	for _, level := range snapshot.Levels() {
		actualPrices[level.Ticker] = append(actualPrices[level.Ticker], level.PreviewBuyPriceNano)
		actualLots[level.Ticker] = append(actualLots[level.Ticker], level.PreviewBuyLots)
	}
	for ticker, expected := range expectedPrices {
		if !slices.Equal(actualPrices[ticker], expected) {
			t.Fatalf("%s prices = %v, want %v", ticker, actualPrices[ticker], expected)
		}
		if !slices.Equal(actualLots[ticker], expectedLots[ticker]) {
			t.Fatalf("%s lots = %v, want %v", ticker, actualLots[ticker], expectedLots[ticker])
		}
	}

	levels := snapshot.Levels()
	levels[0].Ticker = "MUTATED"
	if snapshot.Levels()[0].Ticker == "MUTATED" {
		t.Fatal("snapshot levels are mutable through returned slice")
	}
}

func TestPublishedSnapshotIgnoresSourceRowOrderForHash(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	configRows := readCSV(t, "testdata/BOT_CONFIG.csv")
	reordered := cloneRows(configRows)
	slices.Reverse(reordered[1:])
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows, controlRows},
		ConfigRange:  {configRows, reordered},
	}})

	if _, err := loader.PublishedSnapshot(context.Background()); err != nil {
		t.Fatalf("reordered rows changed normalized snapshot: %v", err)
	}
}

func TestPublishedSnapshotRejectsMutationWithinRevision(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	configRows := readCSV(t, "testdata/BOT_CONFIG.csv")
	mutated := cloneRows(configRows)
	mutated[1][18] = "changed comment"
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows, controlRows},
		ConfigRange:  {configRows, mutated},
	}})

	_, err := loader.PublishedSnapshot(context.Background())
	if !errors.Is(err, ErrRevisionMutated) {
		t.Fatalf("error = %v, want ErrRevisionMutated", err)
	}
}

func TestPublishedSnapshotRejectsChangingRevision(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	configRows := readCSV(t, "testdata/BOT_CONFIG.csv")
	nextControl := cloneRows(controlRows)
	nextControl[2][1] = "13"
	nextConfig := cloneRows(configRows)
	for index := 1; index < len(nextConfig); index++ {
		nextConfig[index][0] = "13"
	}
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows, nextControl},
		ConfigRange:  {configRows, nextConfig},
	}})

	_, err := loader.PublishedSnapshot(context.Background())
	if !errors.Is(err, ErrUnstablePublication) {
		t.Fatalf("error = %v, want ErrUnstablePublication", err)
	}
}

func TestPublishedSnapshotRejectsForbiddenSellScope(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	configRows := readCSV(t, "testdata/BOT_CONFIG.csv")
	configRows[1][8] = "entire_position"
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows},
		ConfigRange:  {configRows},
	}})

	_, err := loader.PublishedSnapshot(context.Background())
	var contractErr *ContractError
	if !errors.As(err, &contractErr) || contractErr.Code != "ENTIRE_POSITION_FORBIDDEN" {
		t.Fatalf("error = %v, want ENTIRE_POSITION_FORBIDDEN", err)
	}
}

func TestContractErrorDoesNotExposeCellValue(t *testing.T) {
	t.Parallel()

	controlRows := readCSV(t, "testdata/BOT_CONTROL.csv")
	secret := "sensitive-account-value"
	controlRows = append(controlRows, []string{"account_id", secret})
	loader := testLoader(&scriptedReader{responses: map[string][][][]string{
		ControlRange: {controlRows},
	}})

	_, err := loader.PublishedSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected unknown field error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("contract error exposes cell value: %v", err)
	}
}

type scriptedReader struct {
	responses map[string][][][]string
	calls     map[string]int
}

func (reader *scriptedReader) ReadRange(_ context.Context, rangeName string) ([][]string, error) {
	if reader.calls == nil {
		reader.calls = make(map[string]int)
	}
	call := reader.calls[rangeName]
	reader.calls[rangeName]++
	responses := reader.responses[rangeName]
	if call >= len(responses) {
		return nil, errors.New("unexpected range read")
	}
	return cloneRows(responses[call]), nil
}

func testLoader(reader Reader) *Loader {
	loader := NewLoader(reader)
	loader.now = func() time.Time {
		return time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC)
	}
	return loader
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open fixture %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return rows
}

func cloneRows(rows [][]string) [][]string {
	cloned := make([][]string, len(rows))
	for index := range rows {
		cloned[index] = slices.Clone(rows[index])
	}
	return cloned
}
