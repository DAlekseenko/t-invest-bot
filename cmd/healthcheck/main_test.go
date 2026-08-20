package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestCheckAcceptsReadyEndpoint(t *testing.T) {
	t.Parallel()

	client := clientReturning(http.StatusOK)
	if err := checkWithClient(context.Background(), client, "http://healthcheck.test/readyz"); err != nil {
		t.Fatalf("check ready endpoint: %v", err)
	}
}

func TestCheckRejectsNotReadyEndpoint(t *testing.T) {
	t.Parallel()

	err := checkWithClient(
		context.Background(),
		clientReturning(http.StatusServiceUnavailable),
		"http://healthcheck.test/readyz",
	)
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("error = %v, want status 503", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func clientReturning(status int) *http.Client {
	return &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}
}
