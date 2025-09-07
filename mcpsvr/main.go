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
	"strings"
	"time"

	"github.com/JeffreyRichter/mcpsvr/config"
	v20250808 "github.com/JeffreyRichter/mcpsvr/v20250808"
	"github.com/JeffreyRichter/svrcore"
	"github.com/JeffreyRichter/svrcore/policies"
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

	policies := []svrcore.Policy{
		shutdownMgr.NewPolicy(),
		newApiVersionSimulatorPolicy("api-version"),
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
	avis := []*svrcore.ApiVersionInfo{
		{ApiVersion: "", BaseApiVersion: "", GetRoutes: noApiVersionRoutes},
		{ApiVersion: "2025-08-08", BaseApiVersion: "", GetRoutes: v20250808.Routes},
	}

	s := &http.Server{
		Handler: svrcore.BuildHandler(svrcore.BuildHandlerConfig{
			Policies:              policies,
			ApiVersionInfos:       avis,
			ApiVersionKeyName:     "api-version",
			ApiVersionKeyLocation: svrcore.ApiVersionKeyLocationHeader,
			Logger:                slog.Default(),
		}),
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

func noApiVersionRoutes(baseRoutes svrcore.ApiVersionRoutes) svrcore.ApiVersionRoutes {
	// If no base api-version, baseRoutes == nil; build routes from scratch

	// Use the patterns below to MODIFY the base's routes (or ignore baseRoutes to build routes from scratch):
	// To existing URL, add/overwrite HTTP method: baseRoutes["<ExistinUrl>"]["<ExistingOrNewHttpMethod>"] = postFoo
	// To existing URL, remove HTTP method:        delete(baseRoutes["<ExistingUrl>"], "<ExisitngHttpMethod>")
	// Remove existing URL entirely:               delete(baseRoutes, "<ExistingUrl>")
	return svrcore.ApiVersionRoutes{
		"/debug/health": map[string]*svrcore.MethodInfo{
			"GET": {Policy: shutdownMgr.HealthProbe},
		},
		"/debug/pprof": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) error { pprof.Index(rr.RW, rr.R); return nil }},
		},
		"/debug/cmdline": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) error { pprof.Cmdline(rr.RW, rr.R); return nil }},
		},
		"/debug/profile": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) error { pprof.Profile(rr.RW, rr.R); return nil }},
		},
		"/debug/symbol": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) error { pprof.Symbol(rr.RW, rr.R); return nil }},
		},
		"/debug/trace": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) error { pprof.Trace(rr.RW, rr.R); return nil }},
		},
	}
}

func newApiVersionSimulatorPolicy(key string) svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		if !strings.HasPrefix(r.R.URL.Path, "/debug/") {
			r.R.Header.Set(key, "2025-08-08")
		}
		return r.Next(ctx)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
