// For webhooks: To test on Internet (https://localhost.run/): ssh-keygen and then ssh -R 80:localhost:8080 localhost.run
// For quality testing: https://dotnetfoundation.org/projects/project-detail/dev-proxy
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"time"

	"github.com/JeffreyRichter/mcpsvc/config"
	v20250808 "github.com/JeffreyRichter/mcpsvc/v20250808"

	si "github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/policies"
)

func main() {
	key := ""
	addr := ":8080"
	startMsg := "Listening on " + addr
	if config.Get().Local {
		b := make([]byte, 16)
		_, _ = rand.Read(b) // guaranteed to return len(b), nil
		key = fmt.Sprintf("%x", b)
		n, _ := rand.Int(rand.Reader, big.NewInt(60000)) // never returns an error
		addr = fmt.Sprintf(":%d", n.Int64()+1025)
		port, err := strconv.Atoi(addr[1:])
		if err != nil {
			fmt.Println("Error generating port:", err)
			os.Exit(1)
		}
		startMsg = fmt.Sprintf(`{"key":%q,"port":%d}`, key, port)
	}

	policies := []si.Policy{
		// Add support for https://shopify.engineering/building-resilient-payment-systems (See "4. Add Monitoring and Alerting")
		policies.NewGracefulShutdownPolicy(), // Incorporate?: https://github.com/enrichman/httpgrace
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
		Addr:           addr,
		Handler:        si.BuildHandler(policies, avis, time.Minute*4),
		MaxHeaderBytes: http.DefaultMaxHeaderBytes,
	}
	fmt.Println(startMsg)
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
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
