package policies

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
func (sm *ShutdownMgr) HealthProbe(ctx context.Context, r *svrcore.ReqRes) error {
	// https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-custom-probe-overview
	if sm.ShuttingDown() {
		return r.WriteError(http.StatusServiceUnavailable, nil, nil, "Service instance shutting down", "This service instance is shutting down. Please try again.")
	}
	return r.WriteSuccess(http.StatusOK, nil, nil, nil)
}

// ShutdownMgrConfig holds the configuration for the shutdown policy.
type ShutdownMgrConfig struct {
	Logger *slog.Logger
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
			c.Logger.Info("Signal " + sig.String() + ": Service instance shutting down")
			// 1. Set flag indicating that shutdown has been requested (health probe uses this to notify load balancer to take node out of rotation)
			sm.shuttingDown.Store(true) // All future requests immedidately return http.StatusServiceUnavailable via this policy

			// 2. Give some time for health probe/load balancer to stop sending traffic to this node
			time.Sleep(c.HealthProbeDelay)

			// 3. Give some time to cancel any remaining in-flight requests
			sm.ctxCancel(errors.New("shutdown requested")) // Cancel the after-inflight-requests context
			time.Sleep(c.CancellationDelay)

			// 4. No more time given, force node shutdown
			c.Logger.Info("All inflight requests complete: Service instance shutting down")
			os.Exit(1) // Kill this service instance
		}
	}()
	return sm
}

// NewPolicy creates a new shutdown policy using ShutdownMgr.
// This policy returns a 503-ServiceUnavailable if the service is shutting down; else the request is processed normally.
func (sm *ShutdownMgr) NewPolicy() svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		if sm.ShuttingDown() {
			return r.WriteError(http.StatusServiceUnavailable, nil, nil, "Server unavailable", "This server instance is shutting down. Please try again.")
		}
		sm.inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
		defer sm.inflightRequests.Done()
		return r.Next(ctx)
	}
}
