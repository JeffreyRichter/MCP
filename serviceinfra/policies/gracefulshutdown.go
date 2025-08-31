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
	context.Context  // Canceled some time after health probe notifies load balancer to remove node
	shuttingDown     atomic.Bool
	inflightRequests sync.WaitGroup // We could Wait() to block until no more inflight requests but we can't hang forever if a request is in a infinite loop
	ctxCancel        context.CancelCauseFunc
}

type ShutdownConfig struct {
	Logger           *slog.Logger
	HealthProbeDelay time.Duration
	CancelDelay      time.Duration
}

// NewShutdownCtx creates a new ShutdownCtx. healthProbeDelay indicates how long to accept more incoming requests
// until the load balancer (hopefully) stops sending traffic. cancelDelay indicates how long to wait
// after attempting graceful shutdown before forceably terminating the process.
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
			time.Sleep(c.CancelDelay)

			// 4. No more time given, force node shutdown
			c.Logger.Info("All inflight requests complete: Service instance shutting down")
			os.Exit(1) // Kill this service instance
		}
	}()
	return s
}

func NewGracefulShutdownPolicy(s *ShutdownCtx) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if s.shuttingDown.Load() {
			return r.Error(http.StatusServiceUnavailable, "Service instance shutting down", "This service instance is shutting down. Please try again.")
		}
		s.inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
		defer s.inflightRequests.Done()
		return r.Next(ctx)
	}
}
