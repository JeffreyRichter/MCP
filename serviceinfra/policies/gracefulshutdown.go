package policies

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

type ShutdownMgr struct {
	shutdownRequested              atomic.Bool
	inflightRequests               sync.WaitGroup
	CtxImmediate                   context.Context // Canceled as soon as the app is requested to shut down
	ctxImmediateCancel             context.CancelCauseFunc
	CtxAfterInflightRequests       context.Context // Canceled right after all inflight requests are complete
	ctxAfterInflightRequestsCancel context.CancelCauseFunc
}

// NewShutdownMgr creates a new ShutdownMgr. healthProbeDelay indicates how long to accept more incoming requests until
// the load balancer (hopefully) stops sending traffic, and delayAfterInflightRequests indicates how long to wait
// after all inflight requests are complete before terminateing the process.
func NewShutdownMgr(healthProbeDelay, delayAfterInflightRequests time.Duration) *ShutdownMgr {
	s := &ShutdownMgr{
		shutdownRequested: atomic.Bool{},
		inflightRequests:  sync.WaitGroup{},
	}
	s.CtxImmediate, s.ctxImmediateCancel = context.WithCancelCause(context.Background())
	s.CtxAfterInflightRequests, s.ctxAfterInflightRequestsCancel = context.WithCancelCause(context.Background())

	go func() {
		// Listen for shutdown signals (e.g., SIGINT, SIGTERM)
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Register the signals we want the channel to receive
		sig := <-sigs                                        // Block until signal is received

		s.shutdownRequested.Store(true)                                                           // All future requests immedidately return http.StatusServiceUnavailable via this policy
		s.ctxImmediateCancel(errors.New("shutdown requested (in-flight requests still running)")) // Cancel the immediate shutdown context
		fmt.Println("Signal " + sig.String() + ": Service instance shutting down")
		// Graceful shutdown logic here
		// TODO: Return non-200 from health probe URL endpoint
		time.Sleep(healthProbeDelay)                                                                    // Give load balancers a chance to stop new traffic from coming in
		s.inflightRequests.Wait()                                                                       // Block until there are no more inflight requests
		s.ctxAfterInflightRequestsCancel(errors.New("shutdown requested (inflight requests complete)")) // Cancel the after-inflight-requests context
		time.Sleep(delayAfterInflightRequests)                                                          // Give any final requests a chance to finish
		fmt.Println("All inflight requests complete: Service instance shutting down")
		os.Exit(1) // Kill this service instance
	}()
	return s
}

func (s *ShutdownMgr) ShutdownRequested() bool { return s.shutdownRequested.Load() }

func (s *ShutdownMgr) IncrementInflightRequests() {
	s.inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
}

func (s *ShutdownMgr) DecrementInflightRequests() {
	s.inflightRequests.Done() // Subtract 1 from wait group whenever a request is done processing
}

func NewGracefulShutdownPolicy(s *ShutdownMgr) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if !s.ShutdownRequested() {
			s.IncrementInflightRequests()
			err := r.Next(ctx)
			s.DecrementInflightRequests()
			return err
		}
		return r.Error(http.StatusServiceUnavailable, "Service instance shutting down", "This service instance is shutting down. Please try again.")
	}
}
