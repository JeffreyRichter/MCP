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
	"time"

	"github.com/JeffreyRichter/mcpsvc/config"
	v20250808 "github.com/JeffreyRichter/mcpsvc/v20250808"
	"github.com/JeffreyRichter/serviceinfra"
	"github.com/JeffreyRichter/serviceinfra/policies"
)

var (
	logger      = slog.Default()
	shutdownMgr = policies.NewShutdownMgr(policies.ShutdownMgrConfig{
		Logger:            logger,
		HealthProbeDelay:  time.Second * 2,
		CancellationDelay: time.Second * 3,
	})
)

func main() {
	key := ""
	port := "8080"
	if config.Get().Local {
		b := make([]byte, 16)
		_, _ = rand.Read(b) // guaranteed to return len(b), nil
		key = fmt.Sprintf("%x", b)
		port = "0" // let the OS choose a port
	}

	policies := []serviceinfra.Policy{
		shutdownMgr.NewPolicy(),
		policies.NewRequestLogPolicy(logger),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthorizationPolicy(key),
		policies.NewMetricsPolicy(logger),
		policies.NewDistributedTracing(),
	}

	// Supported scenarios:
	// 1. New preview/GA version from scratch (fresh or override breaking url/methods)
	// 2. New preview/GA version based on existing preview/GA version
	// 3. Retire old preview/GA version
	avis := []*serviceinfra.ApiVersionInfo{
		// TODO: implement versioning; the below effectively makes versioning optional
		// {ApiVersion: "", BaseApiVersion: "", GetRoutes: noApiVersionRoutes},
		// {ApiVersion: "2025-08-08", BaseApiVersion: "", GetRoutes: v20250808.Routes},
		{ApiVersion: "", BaseApiVersion: "", GetRoutes: v20250808.Routes},
	}

	s := &http.Server{
		Handler:                      serviceinfra.BuildHandler(policies, avis, "api-version", serviceinfra.APIVersionKeyLocationQuery),
		DisableGeneralOptionsHandler: true,
		MaxHeaderBytes:               http.DefaultMaxHeaderBytes,
		BaseContext:                  func(_ net.Listener) context.Context { return shutdownMgr.Context },
		ReadHeaderTimeout:            5 * time.Second,
		ReadTimeout:                  30 * time.Second,
		WriteTimeout:                 30 * time.Second,
	}

	ln := must(net.Listen("tcp", ":"+port))
	var err error
	if _, port, err = net.SplitHostPort(ln.Addr().String()); err != nil {
		panic(err)
	}
	startMsg := fmt.Sprintf("Listening on port: %s", port)
	if config.Get().Local {
		startMsg = fmt.Sprintf(`{"key":%q,"port":%s}`, key, port)
	}
	fmt.Println(startMsg)

	if err := s.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func noApiVersionRoutes(baseRoutes serviceinfra.ApiVersionRoutes) serviceinfra.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return serviceinfra.ApiVersionRoutes{
		"/health": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: shutdownMgr.HealthProbe},
		},
		"/debug/pprof": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *serviceinfra.ReqRes) error { pprof.Index(rr.RW, rr.R); return nil }},
		},
		"/debug/cmdline": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *serviceinfra.ReqRes) error { pprof.Cmdline(rr.RW, rr.R); return nil }},
		},
		"/debug/profile": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *serviceinfra.ReqRes) error { pprof.Profile(rr.RW, rr.R); return nil }},
		},
		"/debug/symbol": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *serviceinfra.ReqRes) error { pprof.Symbol(rr.RW, rr.R); return nil }},
		},
		"/debug/trace": map[string]*serviceinfra.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *serviceinfra.ReqRes) error { pprof.Trace(rr.RW, rr.R); return nil }},
		},
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
