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

	"github.com/JeffreyRichter/serviceinfra"
)

type ShutdownCtx struct {
	// Context canceled after Config.HealthProbeDelay notifies load balancer to remove node
	context.Context
	shuttingDown     atomic.Bool
	inflightRequests sync.WaitGroup          // We could Wait() to block until no more inflight requests but we can't hang forever if a request is in a infinite loop
	ctxCancel        context.CancelCauseFunc // Cancels Context
}

// ShuttingDown returns true after this processes receives a signal to shutdown.
func (s *ShutdownCtx) ShuttingDown() bool {
	return s.shuttingDown.Load()
}

// HealthProbe can be called in response to an HTTP GET. It returns 503-ServiceUnavailable if
// the service is shutting down; else 200-OK.
func (s *ShutdownCtx) HealthProbe(ctx context.Context, r *serviceinfra.ReqRes) error {
	// https://learn.microsoft.com/en-us/azure/load-balancer/load-balancer-custom-probe-overview
	if s.ShuttingDown() {
		return r.Error(http.StatusServiceUnavailable, "Service instance shutting down", "This service instance is shutting down. Please try again.")
	}
	return r.WriteResponse(nil, nil, http.StatusOK, nil)
}

// ShutdownConfig holds the configuration for the shutdown policy.
type ShutdownConfig struct {
	Logger *slog.Logger
	// HealthProbeDelay indicates the time the load balancer takes to stop sending traffic to the process.
	// After this delay, all operations using ShutdownCtx are canceled.
	HealthProbeDelay time.Duration
	// CancellationDelay indicates the time to wait after ShutdownCtx is canceled before forcefully terminating the process
	CancellationDelay time.Duration
}

// NewShutdownCtx creates a new ShutdownCtx using the passed-in ShutdownConfig.
func NewShutdownCtx(c ShutdownConfig) *ShutdownCtx {
	s := &ShutdownCtx{
		shuttingDown:     atomic.Bool{},
		inflightRequests: sync.WaitGroup{},
	}
	s.Context, s.ctxCancel = context.WithCancelCause(context.Background())

	go func() {
		// Listen for shutdown signals (e.g., SIGINT, SIGTERM)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Register the signals we want the channel to receive
		switch sig := <-sigs; sig {                          // Block until signal is received
		case syscall.SIGINT, syscall.SIGTERM: // SIGINT is Ctrl-C, SIGTERM is default termination signal
			c.Logger.Info("Signal " + sig.String() + ": Service instance shutting down")
			// 1. Set flag indicating that shutdown has been requested (health probe uses this to notify load balancer to take node out of rotation)
			s.shuttingDown.Store(true) // All future requests immedidately return http.StatusServiceUnavailable via this policy

			// 2. Give some time for health probe/load balancer to stop sending traffic to this node
			time.Sleep(c.HealthProbeDelay)

			// 3. Give some time to cancel any remaining in-flight requests
			s.ctxCancel(errors.New("shutdown requested")) // Cancel the after-inflight-requests context
			time.Sleep(c.CancellationDelay)

			// 4. No more time given, force node shutdown
			c.Logger.Info("All inflight requests complete: Service instance shutting down")
			os.Exit(1) // Kill this service instance
		}
	}()
	return s
}

// NewShutdownPolicy creates a new shutdown policy using the passed-in ShutdownCtx.
// This policy returns a 503-ServiceUnavailable if the service is shutting down; else the request is processed normally.
func NewShutdownPolicy(s *ShutdownCtx) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if s.ShuttingDown() {
			return r.Error(http.StatusServiceUnavailable, "Service instance shutting down", "This service instance is shutting down. Please try again.")
		}
		s.inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
		defer s.inflightRequests.Done()
		return r.Next(ctx)
	}
}
