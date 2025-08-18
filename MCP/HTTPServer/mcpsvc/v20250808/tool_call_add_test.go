package v20250808

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	si "github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/policies"
)

var ctx = context.Background()

func testServer(t *testing.T) *httptest.Server {
	SetupMockStore(t)

	policies := []si.Policy{
		policies.NewGracefulShutdownPolicy(),
		policies.NewLoggingPolicy(os.Stderr),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthenticationPolicy(),
		policies.NewMetricsPolicy(),
		policies.NewDistributedTracing(),
	}
	avis := []*si.ApiVersionInfo{{GetRoutes: Routes}}
	handler := si.BuildHandler(policies, avis, 5*time.Second)

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv
}

func TestToolCallAdd(t *testing.T) {
	srv := testServer(t)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, srv.URL+"/mcp/tools/add/calls/test-123", strings.NewReader(`{"x":5,"y":3}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	add := struct{ Result AddToolCallResult }{}
	err = json.Unmarshal(b, &add)
	if err != nil {
		t.Fatal(err)
	}

	if got := add.Result.Sum; got != 8 {
		t.Fatalf("expected sum: 8, got %d", got)
	}
}
