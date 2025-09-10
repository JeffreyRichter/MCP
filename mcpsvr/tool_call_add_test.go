package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

var ctx = context.Background()

func TestToolCallAdd(t *testing.T) {
	srv := testServer(t)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/mcp/tools/add/calls/test-123", strings.NewReader(`{"x":5,"y":3}`))
	if isError(err) {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if isError(err) {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if actual := len(resp.Header[http.CanonicalHeaderKey("content-type")]); actual != 1 {
		t.Fatalf("expected 1 content-type, got %d", actual)
	}
	if actual := resp.Header.Get("Content-Type"); actual != "application/json" {
		t.Fatalf("expected application/json, got %q", actual)
	}

	b, err := io.ReadAll(resp.Body)
	if isError(err) {
		t.Fatal(err)
	}
	resp.Body.Close()
	add := struct{ Result AddToolCallResult }{}
	err = json.Unmarshal(b, &add)
	if isError(err) {
		t.Fatal(err)
	}

	if got := add.Result.Sum; got != 8 {
		t.Fatalf("expected sum: 8, got %d", got)
	}
}
