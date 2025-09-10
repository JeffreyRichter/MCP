package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JeffreyRichter/svrcore"
	"github.com/JeffreyRichter/svrcore/policies"
)

var testSvr *mcpPolicies = newLocalMcpPolicies(context.Background(), slog.Default())

func testServer(t *testing.T) *httptest.Server {
	logger := slog.Default()

	policies := []svrcore.Policy{
		policies.NewShutdownMgr(policies.ShutdownMgrConfig{ErrorLogger: logger, HealthProbeDelay: time.Second * 3, CancellationDelay: time.Second * 2}).NewPolicy(),
		policies.NewRequestLogPolicy(logger),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthorizationPolicy(""),
		policies.NewMetricsPolicy(logger),
		policies.NewDistributedTracing(),
	}
	avis := []*svrcore.ApiVersionInfo{{GetRoutes: testSvr.Routes20250808}}
	handler := svrcore.BuildHandler(
		svrcore.BuildHandlerConfig{
			Policies:              policies,
			ApiVersionInfos:       avis,
			ApiVersionKeyName:     "api-version",
			ApiVersionKeyLocation: svrcore.ApiVersionKeyLocationHeader,
			Logger:                slog.Default(),
		})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

type testClient struct {
	t   *testing.T
	url string
}

func newTestClient(t *testing.T) *testClient {
	srv := testServer(t)
	t.Cleanup(srv.Close)
	return &testClient{t: t, url: srv.URL}
}

func (c *testClient) Put(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPut, path, headers, body)
}

func (c *testClient) Post(path string, headers http.Header, body io.Reader) *http.Response {
	return c.do(http.MethodPost, path, headers, body)
}

func (c *testClient) Get(path string, headers http.Header) *http.Response {
	return c.do(http.MethodGet, path, headers, nil)
}

func (c *testClient) do(method, path string, headers http.Header, body io.Reader) *http.Response {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if isError(err) {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		for _, val := range v {
			req.Header.Add(k, val)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if isError(err) {
		c.t.Fatal(err)
	}
	return resp
}
