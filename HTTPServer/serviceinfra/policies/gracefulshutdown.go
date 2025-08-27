package policies

import (
	"context"
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

func NewGracefulShutdownPolicy() serviceinfra.Policy {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM) // Register the signals we want the channel to receive
	shutdownRequested := atomic.Bool{}
	inflightRequests := sync.WaitGroup{} // Initialize a wait group to keep track of all in-flight requests
	go func() {
		sig := <-sigs                 // Block until signal is received
		shutdownRequested.Store(true) // All future requests immedidately return http.StatusServiceUnavailable via this policy
		fmt.Println("Signal " + sig.String() + ": Service instance shutting down")
		// Graceful shutdown logic here
		// TODO: Return non-200 from health probe URL endpoint
		time.Sleep(time.Second * 3) // Give load balancers a chance to stop new traffic from coming in
		inflightRequests.Wait()     // Block until there are no more inflight requests
		os.Exit(1)                  // Kill this service instance
	}()

	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if !shutdownRequested.Load() {
			inflightRequests.Add(1) // Add 1 to wait group whenever a new request comes into the service
			err := r.Next(ctx)
			inflightRequests.Done() // Subtract 1 from wait group whenever a request is done processing
			return err
		}
		return r.Error(http.StatusServiceUnavailable, "Service instance shutting down", "This service instance is shutting down. Please try again.")
	}
}
