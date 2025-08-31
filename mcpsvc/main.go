// For webhooks: To test on Internet (https://localhost.run/): ssh-keygen and then ssh -R 80:localhost:8080 localhost.run
// For quality testing: https://dotnetfoundation.org/projects/project-detail/dev-proxy
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"time"

	"github.com/JeffreyRichter/mcpsvc/config"
	v20250808 "github.com/JeffreyRichter/mcpsvc/v20250808"
	si "github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/policies"
)

var shutdownCtx = policies.NewShutdownCtx(policies.ShutdownConfig{Logger: slog.Default(), HealthProbeDelay: time.Second * 2, CancelDelay: time.Second * 3})

func main() {
	key := ""
	port := "8080"
	if config.Get().Local {
		b := make([]byte, 16)
		_, _ = rand.Read(b) // guaranteed to return len(b), nil
		key = fmt.Sprintf("%x", b)
		port = "0" // let the OS choose a port
	}

	policies := []si.Policy{
		// Add support for https://shopify.engineering/building-resilient-payment-systems (See "4. Add Monitoring and Alerting")
		policies.NewGracefulShutdownPolicy(shutdownCtx),
		policies.NewLoggingPolicy(os.Stderr),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthorizationPolicy(key),
		policies.NewMetricsPolicy(),
		policies.NewDistributedTracing(),
	}

	// Supported scenarios:
	// 1. New preview/GA version from scratch (fresh or override breaking url/methods)
	// 2. New preview/GA version based on existing preview/GA version
	// 3. Retire old preview/GA version
	avis := []*si.ApiVersionInfo{
		// TODO: implement versioning; the below effectively makes versioning optional
		// {ApiVersion: "", BaseApiVersion: "", GetRoutes: noApiVersionRoutes},
		// {ApiVersion: "2025-08-08", BaseApiVersion: "", GetRoutes: v20250808.Routes},
		{ApiVersion: "", BaseApiVersion: "", GetRoutes: v20250808.Routes},
	}

	s := &http.Server{
		Handler:        si.BuildHandler(policies, avis, time.Minute*4),
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
	}

	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error starting listener:", err)
		os.Exit(1)
	}
	if _, port, err = net.SplitHostPort(ln.Addr().String()); err != nil {
		fmt.Println("Error getting port:", err)
		os.Exit(1)
	}
	startMsg := fmt.Sprintf("Listening on :%s", port)
	if config.Get().Local {
		startMsg = fmt.Sprintf(`{"key":%q,"port":%s}`, key, port)
	}
	fmt.Println(startMsg)

	if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("Error starting server:", err)
		os.Exit(1)
	}
}

func noApiVersionRoutes(baseRoutes si.ApiVersionRoutes) si.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return si.ApiVersionRoutes{
		"/debug/pprof": map[string]*si.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *si.ReqRes) error { pprof.Index(rr.RW, rr.R); return nil }},
		},
		"/debug/cmdline": map[string]*si.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *si.ReqRes) error { pprof.Cmdline(rr.RW, rr.R); return nil }},
		},
		"/debug/profile": map[string]*si.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *si.ReqRes) error { pprof.Profile(rr.RW, rr.R); return nil }},
		},
		"/debug/symbol": map[string]*si.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *si.ReqRes) error { pprof.Symbol(rr.RW, rr.R); return nil }},
		},
		"/debug/trace": map[string]*si.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *si.ReqRes) error { pprof.Trace(rr.RW, rr.R); return nil }},
		},
	}
}
