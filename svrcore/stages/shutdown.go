// TODO: Here's what we want...
// 1. When process receives SIGINT/SIGTERM/(SIGQUIT: Ctrl+\), set flag indicating shutdown started.
// 		HealthProbe() returns 503-ServiceUnavailable if shutdown has been requested; else 200-OK.
// 2. Delay Xxx for load balancer to stop sending traffic.
// 3. After delay, create context (derived from Background) and pass to http.Server's Shutdown method
// 4. Immediately cancel http.Server's BaseContext

package stages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/JeffreyRichter/svrcore"
)

// ShutdownMgr provides a stage that returns 503-ServiceUnavailable if the service is shutting down.
// It also provides a context that is canceled after a delay when shutdown is requested.
// This context can be used as the BaseContext for http.Server to cancel all in-flight requests.
type ShutdownMgr struct {
	// Context canceled after Config.HealthProbeDelay notifies load balancer to remove node
	context.Context
	shuttingDown     atomic.Bool
	inflightRequests sync.WaitGroup          // We could Wait() to block until no more inflight requests but we can't hang forever if a request is in a infinite loop
	ctxCancel        context.CancelCauseFunc // Cancels Context
}

// ShuttingDown returns true after this processes receives a signal to shutdown.
func (sm *ShutdownMgr) ShuttingDown() bool { return sm.shuttingDown.Load() }

// HealthProbe can be called in response to an HTTP GET. It returns 503-ServiceUnavailable if
// the service is shutting down; else 200-OK.
func (sm *ShutdownMgr) HealthProbe(ctx context.Context, r *svrcore.ReqRes) bool {
	// https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-custom-probe-overview
	if sm.ShuttingDown() {
		return r.WriteError(http.StatusServiceUnavailable, nil, nil, "Service instance shutting down", "This service instance is shutting down. Please try again.")
	}
	return r.WriteSuccess(http.StatusOK, nil, nil, nil)
}

// ShutdownMgrConfig holds the configuration for the shutdown stage.
type ShutdownMgrConfig struct {
	ErrorLogger *slog.Logger
	// HealthProbeDelay indicates the time the load balancer takes to stop sending traffic to the process.
	// After this delay, all operations using ShutdownMgr are canceled.
	HealthProbeDelay time.Duration
	// CancellationDelay indicates the time to wait after ShutdownMgr is canceled before forcefully terminating the process
	CancellationDelay time.Duration
}

// NewShutdownMgr creates a new ShutdownMgr using the passed-in ShutdownConfig.
// You can set http.Serve's BaseContext to `func(_ net.Listener) context.Context { return shutdownCtx }`.
func NewShutdownMgr(c ShutdownMgrConfig) *ShutdownMgr {
	sm := &ShutdownMgr{shuttingDown: atomic.Bool{}, inflightRequests: sync.WaitGroup{}}
	sm.Context, sm.ctxCancel = context.WithCancelCause(context.Background())

	go func() {
		// Listen for shutdown signals (e.g., SIGINT, SIGTERM)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Register the signals we want the channel to receive
		switch sig := <-sigs; sig {                          // Block until signal is received
		case syscall.SIGINT, syscall.SIGTERM: // SIGINT is Ctrl-C, SIGTERM is default termination signal
			c.ErrorLogger.LogAttrs(sm.Context, slog.LevelInfo,
				"Server shutdown start", slog.String("signal", sig.String()))
			// 1. Set flag indicating that shutdown has been requested (health probe uses this to notify load balancer to take node out of rotation)
			sm.shuttingDown.Store(true) // All future requests immedidately return http.StatusServiceUnavailable via this stage

			// 2. Give some time for health probe/load balancer to stop sending traffic to this node
			time.Sleep(c.HealthProbeDelay)

			// 3. Give some time to cancel any remaining in-flight requests
			sm.ctxCancel(errors.New("shutdown requested")) // Cancel the after-inflight-requests context
			time.Sleep(c.CancellationDelay)

			// 4. No more time given, force node shutdown
			c.ErrorLogger.LogAttrs(sm.Context, slog.LevelInfo, "Server shutdown complete")
			os.Exit(1) // Kill this service instance
		}
	}()
	return sm
}

// NewStage creates a new shutdown stage using ShutdownMgr.
// This stage returns a 503-ServiceUnavailable if the service is shutting down; else the request is processed normally.
func (sm *ShutdownMgr) NewStage() svrcore.Stage {
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if sm.ShuttingDown() {
			return r.WriteError(http.StatusServiceUnavailable, nil, nil, "Server unavailable", "This server instance is shutting down. Please try again.")
		}
		sm.inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
		defer sm.inflightRequests.Done()
		return r.Next(ctx)
	}
}
