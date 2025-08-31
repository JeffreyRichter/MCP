package policies

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewMetricsPolicy(logger *slog.Logger) serviceinfra.Policy {
	requestCountPerMinute := newRateCounter(time.Minute)
	requestLatencyPerMinute := newRateCounter(time.Minute)
	requestServiceFailuresPerMinute := newRateCounter(time.Minute)

	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		// Add support for https://shopify.engineering/building-resilient-payment-systems (See "4. Add Monitoring and Alerting")
		// Google’s site reliability engineering (SRE) book lists four golden signals a user-facing system should be monitored for:
		requestCountPerMinute.Add(1) // Traffic: the rate in which new work comes into the system, typically expressed in requests per minute.

		// Saturation: how much load the system is under, relative to its total capacity. This could be the amount of memory used versus available or a thread pool’s active threads versus total number of threads available, in any layer of the system.
		var latestMemStats runtime.MemStats
		runtime.ReadMemStats(&latestMemStats) // TODO: Log memory metrics
		logger.Info("Memory Stats", "Alloc", latestMemStats.Alloc, "TotalAlloc", latestMemStats.TotalAlloc, "Sys", latestMemStats.Sys, "NumGC", latestMemStats.NumGC)

		latestNumGoroutine := runtime.NumGoroutine() // TODO: Log # of goroutines?
		logger.Info("Goroutine Count", "Count", latestNumGoroutine)

		start := time.Now()
		err := r.Next(ctx)
		duration := time.Since(start) // Latency: the amount of time it takes to process a unit of work, broken down between success and failures.
		requestLatencyPerMinute.Add(duration.Milliseconds())
		var se *serviceinfra.ServiceError
		if err != nil && errors.As(err, &se) && (se.StatusCode >= 500 && se.StatusCode < 600) {
			requestServiceFailuresPerMinute.Add(1) // Errors: the rate of unexpected service things (5xx) happening.
		}
		logger.Info("Requests", "requests/minute", requestCountPerMinute.Rate(), "request ms/minute", requestLatencyPerMinute.Rate(), "5xx/minute", requestServiceFailuresPerMinute.Rate())
		return err
	}
}
