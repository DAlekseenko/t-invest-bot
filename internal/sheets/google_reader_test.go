package sheets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestGoogleReaderReadsFormattedRows(t *testing.T) {
	t.Parallel()

	reader := testGoogleReader(t, func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if !strings.Contains(request.URL.Path, "/v4/spreadsheets/sheet-id/values/") {
			t.Errorf("path = %s, want values.get path", request.URL.Path)
		}
		assertQuery(t, request.URL.Query(), "majorDimension", "ROWS")
		assertQuery(t, request.URL.Query(), "valueRenderOption", formattedValue)
		assertQuery(t, request.URL.Query(), "fields", "values")
		return jsonResponse(`{
            "values": [["key", "value"], ["published_revision", "12"]]
        }`), nil
	})

	rows, err := reader.ReadRange(context.Background(), ControlRange)
	if err != nil {
		t.Fatalf("read range: %v", err)
	}
	want := [][]string{{"key", "value"}, {"published_revision", "12"}}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", rows, want)
	}
	for index := range want {
		if strings.Join(rows[index], "|") != strings.Join(want[index], "|") {
			t.Fatalf("row %d = %v, want %v", index, rows[index], want[index])
		}
	}
}

func TestGoogleReaderRejectsRangeOutsideContract(t *testing.T) {
	t.Parallel()

	requestMade := false
	reader := testGoogleReader(t, func(*http.Request) (*http.Response, error) {
		requestMade = true
		return jsonResponse(`{}`), nil
	})
	_, err := reader.ReadRange(context.Background(), "HiddenSheet!A:Z")
	if err == nil || !strings.Contains(err.Error(), "outside the configuration contract") {
		t.Fatalf("error = %v, want contract range error", err)
	}
	if requestMade {
		t.Fatal("request was made for a range outside the contract")
	}
}

func TestGoogleReaderRejectsNumericAPICell(t *testing.T) {
	t.Parallel()

	reader := testGoogleReader(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(`{"values":[["published_revision",12.5]]}`), nil
	})
	_, err := reader.ReadRange(context.Background(), ControlRange)
	if err == nil || !strings.Contains(err.Error(), "unexpected numeric value") {
		t.Fatalf("error = %v, want numeric cell type error", err)
	}
	if strings.Contains(err.Error(), "12.5") {
		t.Fatalf("error exposes cell value: %v", err)
	}
}

func TestGoogleReaderRejectsHTTPErrorWithoutResponsePayload(t *testing.T) {
	t.Parallel()

	secretPayload := "upstream-sensitive-payload"
	reader := testGoogleReader(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusForbidden,
			Body:       io.NopCloser(strings.NewReader(secretPayload)),
		}, nil
	})
	_, err := reader.ReadRange(context.Background(), ControlRange)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 403") {
		t.Fatalf("error = %v, want HTTP status error", err)
	}
	if strings.Contains(err.Error(), secretPayload) {
		t.Fatalf("error exposes upstream payload: %v", err)
	}
}

func TestGoogleReaderPropagatesCancelledContext(t *testing.T) {
	t.Parallel()

	reader := testGoogleReader(t, func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reader.ReadRange(ctx, ControlRange)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestNewGoogleReaderValidatesRequiredConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewGoogleReader(context.Background(), "", "credentials.json"); err == nil {
		t.Fatal("empty spreadsheet ID accepted")
	}
	if _, err := NewGoogleReader(context.Background(), "sheet-id", ""); err == nil {
		t.Fatal("empty credentials file accepted")
	}
}

func TestRequireServiceAccountDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()

	secret := "credential-secret-value"
	err := requireServiceAccount([]byte(`{"type":"authorized_user","private_key":"` + secret + `"}`))
	if err == nil {
		t.Fatal("non-service-account credentials accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error exposes credential contents: %v", err)
	}
}

func testGoogleReader(
	t *testing.T,
	roundTrip func(*http.Request) (*http.Response, error),
) *GoogleReader {
	t.Helper()
	reader, err := newGoogleReader(
		&http.Client{Transport: roundTripFunc(roundTrip)},
		"sheet-id",
		"https://sheets.test/v4",
	)
	if err != nil {
		t.Fatalf("create test Google reader: %v", err)
	}
	return reader
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func assertQuery(t *testing.T, query url.Values, key, expected string) {
	t.Helper()
	if actual := query.Get(key); actual != expected {
		t.Errorf("query %s = %q, want %q", key, actual, expected)
	}
}
