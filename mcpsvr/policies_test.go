package main

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/JeffreyRichter/internal/aids"
)

func TestListTools(t *testing.T) {
	client := newTestClient(t)
	resp := client.Get("/mcp/tools", http.Header{})

	b, err := io.ReadAll(resp.Body)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	resp.Body.Close()

	actual := struct {
		Tools []map[string]any `json:"tools"`
	}{}
	err = json.Unmarshal(b, &actual)
	if aids.IsError(err) {
		t.Fatal(err)
	}
	if actual := len(actual.Tools); actual != 3 {
		t.Fatalf("expected 3 tools, got %d", actual)
	}

	etag, has := resp.Header[http.CanonicalHeaderKey("etag")]
	if !has {
		t.Fatal("no etag")
	}
	if actual := len(etag); actual != 1 {
		t.Fatalf("wanted 1 etag, got %d", actual)
	}
	if actual := string(etag[0]); actual != "v20250808" {
		t.Fatal("unexpected etag: " + actual)
	}
}

func TestListToolsETag(t *testing.T) {
	t.Run("if-none-match", func(t *testing.T) {
		client := newTestClient(t)
		resp := client.Get("/mcp/tools", http.Header{"If-None-Match": []string{"v20250808"}})
		if resp.StatusCode != http.StatusNotModified {
			t.Fatalf("expected 304 Not Modified, got %d", resp.StatusCode)
		}
	})
	t.Run("if-match", func(t *testing.T) {
		client := newTestClient(t)
		resp := client.Get("/mcp/tools", http.Header{"If-Match": []string{"v20250808"}})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
	})
}
