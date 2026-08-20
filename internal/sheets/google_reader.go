package sheets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
)

const (
	formattedValue            = "FORMATTED_VALUE"
	googleSheetsBaseURL       = "https://sheets.googleapis.com/v4"
	spreadsheetsReadonlyScope = "https://www.googleapis.com/auth/spreadsheets.readonly"
	maxCredentialsSize        = 1 << 20
	maxRangeResponseSize      = 4 << 20
)

// GoogleReader is a read-only adapter for the two ranges in the published
// configuration contract.
type GoogleReader struct {
	client        *http.Client
	spreadsheetID string
	baseURL       string
}

var _ Reader = (*GoogleReader)(nil)

func NewGoogleReader(ctx context.Context, spreadsheetID, credentialsFile string) (*GoogleReader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(spreadsheetID) == "" {
		return nil, errors.New("google spreadsheet ID is required")
	}
	if strings.TrimSpace(credentialsFile) == "" {
		return nil, errors.New("google credentials file is required")
	}

	contents, err := readCredentialsFile(credentialsFile)
	if err != nil {
		return nil, fmt.Errorf("read Google credentials file: %w", err)
	}
	if err := requireServiceAccount(contents); err != nil {
		return nil, err
	}
	oauthConfig, err := google.JWTConfigFromJSON(contents, spreadsheetsReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parse Google service account credentials: %w", err)
	}
	return newGoogleReader(oauthConfig.Client(ctx), spreadsheetID, googleSheetsBaseURL)
}

func newGoogleReader(client *http.Client, spreadsheetID, baseURL string) (*GoogleReader, error) {
	if client == nil {
		return nil, errors.New("google Sheets HTTP client is required")
	}
	if strings.TrimSpace(spreadsheetID) == "" {
		return nil, errors.New("google spreadsheet ID is required")
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, errors.New("google Sheets base URL is invalid")
	}
	return &GoogleReader{
		client:        client,
		spreadsheetID: strings.TrimSpace(spreadsheetID),
		baseURL:       strings.TrimRight(baseURL, "/"),
	}, nil
}

func (reader *GoogleReader) ReadRange(ctx context.Context, rangeName string) ([][]string, error) {
	if reader == nil || reader.client == nil {
		return nil, errors.New("google Sheets reader is required")
	}
	if err := validateRange(rangeName); err != nil {
		return nil, err
	}

	endpoint := reader.baseURL + "/spreadsheets/" + url.PathEscape(reader.spreadsheetID) +
		"/values/" + url.PathEscape(rangeName)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build Google Sheets request: %w", err)
	}
	query := request.URL.Query()
	query.Set("majorDimension", "ROWS")
	query.Set("valueRenderOption", formattedValue)
	query.Set("fields", "values")
	request.URL.RawQuery = query.Encode()

	response, err := reader.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("read Google Sheets range %s: %w", sheetName(rangeName), err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"read Google Sheets range %s: unexpected HTTP status %d",
			sheetName(rangeName),
			response.StatusCode,
		)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxRangeResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read Google Sheets range %s response: %w", sheetName(rangeName), err)
	}
	if len(body) > maxRangeResponseSize {
		return nil, fmt.Errorf("read Google Sheets range %s: response is too large", sheetName(rangeName))
	}
	var payload struct {
		Values [][]json.RawMessage `json:"values"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Google Sheets range %s response: %w", sheetName(rangeName), err)
	}
	rows, err := stringRows(payload.Values)
	if err != nil {
		return nil, fmt.Errorf("decode Google Sheets range %s: %w", sheetName(rangeName), err)
	}
	return rows, nil
}

func readCredentialsFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	contents, err := io.ReadAll(io.LimitReader(file, maxCredentialsSize+1))
	if err != nil {
		return nil, err
	}
	if len(contents) > maxCredentialsSize {
		return nil, errors.New("google credentials file is too large")
	}
	return contents, nil
}

func requireServiceAccount(contents []byte) error {
	var descriptor struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(contents, &descriptor); err != nil {
		return errors.New("google credentials file is not valid JSON")
	}
	if descriptor.Type != "service_account" {
		return errors.New("google credentials must have service_account type")
	}
	return nil
}

func validateRange(rangeName string) error {
	switch rangeName {
	case ControlRange, ConfigRange:
		return nil
	default:
		return errors.New("google Sheets range is outside the configuration contract")
	}
}

func sheetName(rangeName string) string {
	name, _, _ := strings.Cut(rangeName, "!")
	return name
}

func stringRows(values [][]json.RawMessage) ([][]string, error) {
	rows := make([][]string, len(values))
	for rowIndex, sourceRow := range values {
		row := make([]string, len(sourceRow))
		for columnIndex, value := range sourceRow {
			trimmed := bytes.TrimSpace(value)
			if bytes.Equal(trimmed, []byte("null")) {
				continue
			}
			if len(trimmed) == 0 || trimmed[0] != '"' {
				return nil, fmt.Errorf(
					"row %d column %d has unexpected %s value",
					rowIndex+1,
					columnIndex+1,
					jsonValueType(trimmed),
				)
			}
			if err := json.Unmarshal(trimmed, &row[columnIndex]); err != nil {
				return nil, fmt.Errorf("row %d column %d is not a string", rowIndex+1, columnIndex+1)
			}
		}
		rows[rowIndex] = row
	}
	return rows, nil
}

func jsonValueType(value []byte) string {
	if len(value) == 0 {
		return "empty"
	}
	switch value[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "numeric"
	}
}
