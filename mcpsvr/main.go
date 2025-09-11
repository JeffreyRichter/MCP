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

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/resources/azresources"
	"github.com/JeffreyRichter/mcpsvr/resources/localresources"
	"github.com/JeffreyRichter/svrcore"
	"github.com/JeffreyRichter/svrcore/policies"
)

var (
	errorLogger   = slog.Default()
	requestLogger = slog.Default()
	metricsLogger = slog.Default()
	shutdownMgr   = policies.NewShutdownMgr(policies.ShutdownMgrConfig{ErrorLogger: errorLogger, HealthProbeDelay: time.Second * 2, CancellationDelay: time.Second * 3})
)

func main() {
	var c Configuration
	c.Load()

	var p *mcpPolicies
	port, sharedKey := "8080", ""
	switch {
	case c.Local:
		p = newLocalMcpPolicies(shutdownMgr.Context, errorLogger)
		b := [16]byte{}
		_, _ = rand.Read(b[:]) // guaranteed to return len(b), nil
		port, sharedKey = "0", fmt.Sprintf("%x", b)

	case c.AzuriteAccount != "":
		blobCred := aids.Must(azblob.NewSharedKeyCredential(c.AzuriteAccount, c.AzuriteKey))
		blobClient := aids.Must(azblob.NewClientWithSharedKeyCredential(c.AzureBlobURL, blobCred, nil))
		queueCred := aids.Must(azqueue.NewSharedKeyCredential(c.AzuriteAccount, c.AzuriteKey))
		queueClient := aids.Must(azqueue.NewQueueClientWithSharedKeyCredential(c.AzureQueueURL, queueCred, nil))
		p = newAzureMcpPolicies(shutdownMgr.Context, errorLogger, blobClient, queueClient)

	default:
		cred := aids.Must(azidentity.NewDefaultAzureCredential(nil))
		blobClient := aids.Must(azblob.NewClient(c.AzureBlobURL, cred, nil))
		queueClient := aids.Must(azqueue.NewQueueClient(c.AzureQueueURL, cred, nil))
		p = newAzureMcpPolicies(shutdownMgr.Context, errorLogger, blobClient, queueClient)
	}

	policies := []svrcore.Policy{
		shutdownMgr.NewPolicy(),
		newApiVersionSimulatorPolicy(),
		policies.NewRequestLogPolicy(requestLogger),
		policies.NewThrottlingPolicy(100),
		policies.NewAuthorizationPolicy(sharedKey),
		policies.NewMetricsPolicy(metricsLogger),
		policies.NewDistributedTracing(),
	}

	// Supported scenarios:
	// 1. New preview/GA version from scratch (fresh or override breaking url/methods)
	// 2. New preview/GA version based on existing preview/GA version
	// 3. Retire old preview/GA version
	avis := []*svrcore.ApiVersionInfo{
		{ApiVersion: "", BaseApiVersion: "", GetRoutes: noApiVersionRoutes},
		{ApiVersion: "2025-08-08", BaseApiVersion: "", GetRoutes: p.Routes20250808},
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

	ln := aids.Must(net.Listen("tcp", net.JoinHostPort("", port)))
	var err error
	if _, port, err = net.SplitHostPort(ln.Addr().String()); aids.IsError(err) {
		panic(err)
	}
	startMsg := fmt.Sprintf("Listening on port: %s", port)
	if c.Local {
		startMsg = fmt.Sprintf(`{"port":%s, "key":%q}`, port, sharedKey)
	}
	fmt.Println(startMsg)

	if err := s.Serve(ln); aids.IsError(err) && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func newLocalMcpPolicies(shutdownCtx context.Context, errorLogger *slog.Logger) *mcpPolicies {
	ops := &mcpPolicies{errorLogger: errorLogger, store: localresources.NewToolCallStore(shutdownCtx)}
	ops.pm = localresources.NewPhaseMgr(shutdownCtx, localresources.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	ops.buildToolInfos()
	return ops
}

func newAzureMcpPolicies(shutdownCtx context.Context, errorLogger *slog.Logger, blobClient *azblob.Client, queueClient *azqueue.QueueClient) *mcpPolicies {
	ops := &mcpPolicies{errorLogger: errorLogger, store: azresources.NewToolCallStore(blobClient)}
	pm, err := azresources.NewPhaseMgr(shutdownCtx, queueClient, ops.store, azresources.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	aids.AssertSuccess(err)
	ops.pm = pm
	ops.buildToolInfos()
	return ops
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

func newApiVersionSimulatorPolicy() svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		if !strings.HasPrefix(r.R.URL.Path, "/debug/") {
			r.R.Header.Set("api-version", "2025-08-08")
		}
		return r.Next(ctx)
	}
}
