// For webhooks: To test on Internet (https://localhost.run/): ssh-keygen and then ssh -R 80:localhost:8080 localhost.run
// For quality testing: https://dotnetfoundation.org/projects/project-detail/dev-proxy
package main

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azqueue"
	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/toolcall/azure"
	"github.com/JeffreyRichter/mcpsvr/toolcall/local"
	"github.com/JeffreyRichter/svrcore"
	"github.com/JeffreyRichter/svrcore/policies"
)

var (
	errorLogger   = slog.New(slog.NewJSONHandler(os.Stderr, nil))
	metricsLogger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	shutdownMgr   = policies.NewShutdownMgr(policies.ShutdownMgrConfig{ErrorLogger: errorLogger, HealthProbeDelay: time.Second * 2, CancellationDelay: time.Second * 3})
)

func main() {
	var c Configuration
	c.Load()

	var routes *mcpPolicies
	port, sharedKey := "0", "" // Default to OS-port & no sharedKey
	switch {
	case c.Local:
		pid := flag.Int("pid", 0, "Parent process ID. This server shuts down when the parent process exits.")
		flag.Parse()
		if *pid != 0 { // Started by a parent process; shut down when the parent goes away
			b := [16]byte{}
			_, _ = rand.Read(b[:])           // guaranteed to return len(b), nil
			sharedKey = fmt.Sprintf("%x", b) // Random port & sharedKey
			go processWatchdog(*pid, time.Second*5)
		} else {
			port, sharedKey = "8080", "ForDebuggingOnly"
		}
		routes = newLocalMcpPolicies(shutdownMgr.Context, errorLogger)

	case c.AzuriteAccount != "":
		blobCred := aids.Must(azblob.NewSharedKeyCredential(c.AzuriteAccount, c.AzuriteKey))
		blobClient := aids.Must(azblob.NewClientWithSharedKeyCredential(c.AzureBlobURL, blobCred, nil))
		queueCred := aids.Must(azqueue.NewSharedKeyCredential(c.AzuriteAccount, c.AzuriteKey))
		queueClient := aids.Must(azqueue.NewQueueClientWithSharedKeyCredential(c.AzureQueueURL, queueCred, nil))
		routes = newAzureMcpPolicies(shutdownMgr.Context, errorLogger, blobClient, queueClient)

	default:
		cred := aids.Must(azidentity.NewDefaultAzureCredential(nil))
		blobClient := aids.Must(azblob.NewClient(c.AzureBlobURL, cred, nil))
		queueClient := aids.Must(azqueue.NewQueueClient(c.AzureQueueURL, cred, nil))
		routes = newAzureMcpPolicies(shutdownMgr.Context, errorLogger, blobClient, queueClient)
	}

	policies := []svrcore.Policy{
		shutdownMgr.NewPolicy(),
		policies.NewMetricsPolicy(metricsLogger),
		newApiVersionSimulatorPolicy(),
		policies.NewSharedKeyPolicy(sharedKey),
		//policies.NewThrottlingPolicy(100),
		//policies.NewDistributedTracing(),
	}

	// Supported scenarios:
	// 1. New preview/GA version from scratch (fresh or override breaking url/methods)
	// 2. New preview/GA version based on existing preview/GA version
	// 3. Retire old preview/GA version
	avis := []*svrcore.ApiVersionInfo{
		{ApiVersion: "", BaseApiVersion: "", GetRoutes: noApiVersionRoutes},
		{ApiVersion: "2025-08-08", BaseApiVersion: "", GetRoutes: routes.Routes20250808},
	}

	s := &http.Server{
		Handler: svrcore.BuildHandler(svrcore.BuildHandlerConfig{
			Policies:              policies,
			ApiVersionInfos:       avis,
			ApiVersionKeyName:     "Api-Version", // Must be canonicalized HTTP header key
			ApiVersionKeyLocation: svrcore.ApiVersionKeyLocationHeader,
			Logger:                slog.New(slog.NewTextHandler(os.Stdout, nil)),
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
		startMsg = fmt.Sprintf(`{"port":%q, "key":%q}`, port, sharedKey)
	}
	fmt.Println(startMsg)
	os.Stdout.Sync()

	if err := s.Serve(ln); aids.IsError(err) && !errors.Is(err, http.ErrServerClosed) {
		panic(err)
	}
}

func newLocalMcpPolicies(shutdownCtx context.Context, errorLogger *slog.Logger) *mcpPolicies {
	ops := &mcpPolicies{errorLogger: errorLogger, store: local.NewToolCallStore(shutdownCtx)}
	ops.pm = local.NewPhaseMgr(shutdownCtx, local.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	ops.buildToolInfos()
	return ops
}

func newAzureMcpPolicies(shutdownCtx context.Context, errorLogger *slog.Logger, blobClient *azblob.Client, queueClient *azqueue.QueueClient) *mcpPolicies {
	ops := &mcpPolicies{errorLogger: errorLogger, store: azure.NewToolCallStore(blobClient)}
	pm, err := azure.NewPhaseMgr(shutdownCtx, queueClient, ops.store, azure.PhaseMgrConfig{ErrorLogger: errorLogger, ToolNameToProcessPhaseFunc: ops.toolNameToProcessPhaseFunc})
	aids.Must0(err)
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
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) bool { pprof.Index(rr.RW, rr.R); return false }},
		},
		"/debug/cmdline": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) bool { pprof.Cmdline(rr.RW, rr.R); return false }},
		},
		"/debug/profile": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) bool { pprof.Profile(rr.RW, rr.R); return false }},
		},
		"/debug/symbol": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) bool { pprof.Symbol(rr.RW, rr.R); return false }},
		},
		"/debug/trace": map[string]*svrcore.MethodInfo{
			"GET": {Policy: func(ctx context.Context, rr *svrcore.ReqRes) bool { pprof.Trace(rr.RW, rr.R); return false }},
		},
	}
}

func newApiVersionSimulatorPolicy() svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if !strings.HasPrefix(r.R.URL.Path, "/debug/") {
			r.R.Header.Set("api-version", "2025-08-08")
		}
		return r.Next(ctx)
	}
}

// processWatchdog periodically checks to see if the pid process is still alive; if not, it kills this process
// processWatchdog should be started as its own goroutine; it never returns
func processWatchdog(pid int, delayBetween time.Duration) {
	for {
		time.Sleep(delayBetween) // Check to see if parent is still alive periodically
		if p, err := os.FindProcess(pid); aids.IsError(err) {
			os.Exit(1) // Can't find parent process; exit
		} else {
			p.Release()
		}
	}
}

/* Set up the flight recorder: https://go.dev/blog/flight-recorder
fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{
	MinAge:   200 * time.Millisecond,
	MaxBytes: 1 << 20, // 1 MiB
})
fr.Start()*/

// captureSnapshot captures a flight recorder snapshot.
/*func captureSnapshot(fr *trace.FlightRecorder) {
	f, err := os.Create("snapshot_" + time.Now().Format("20060102_150405") + ".trace")
	if err != nil {
		log.Printf("opening snapshot file %s failed: %s", f.Name(), err)
		return
	}
	defer f.Close() // ignore error

	_, err = fr.WriteTo(f)
	if err != nil {
		log.Printf("writing snapshot to file %s failed: %s", f.Name(), err)
		return
	}

	fr.Stop() // Stop the flight recorder after the snapshot has been taken.
	log.Printf("captured a flight recorder snapshot to %s", f.Name())
}*/
