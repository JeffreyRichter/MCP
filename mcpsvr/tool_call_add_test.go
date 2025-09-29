package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/JeffreyRichter/internal/aids"
)

var ctx = context.Background()

func TestToolCallAdd(t *testing.T) {
	srv := testServer(t)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/mcp/tools/add/calls/test-123", strings.NewReader(`{"x":5,"y":3}`))
	if aids.IsError(err) {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", time.Now().Format(time.RFC3339Nano))
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if aids.IsError(err) {
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
	if aids.IsError(err) {
		t.Fatal(err)
	}
	resp.Body.Close()
	add := struct{ Result addToolCallResult }{}
	err = json.Unmarshal(b, &add)
	if aids.IsError(err) {
		t.Fatal(err)
	}

	if got := add.Result.Sum; got != 8 {
		t.Fatalf("expected sum: 8, got %d", got)
	}
}
