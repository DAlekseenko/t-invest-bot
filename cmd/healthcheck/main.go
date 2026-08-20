package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const defaultEndpoint = "http://127.0.0.1:8080/readyz"

func main() {
	endpoint := defaultEndpoint
	if len(os.Args) == 2 {
		endpoint = os.Args[1]
	}
	if err := check(context.Background(), endpoint); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func check(parent context.Context, endpoint string) error {
	return checkWithClient(parent, http.DefaultClient, endpoint)
}

func checkWithClient(parent context.Context, client *http.Client, endpoint string) error {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build readiness request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request readiness: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("readiness returned status %d", response.StatusCode)
	}
	return nil
}
