package stages

import (
	"context"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/JeffreyRichter/svrcore"
)

func NewSharedKeyStage(sharedKey string) svrcore.Stage {
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if sharedKey != "" && (r.R.Header.Get("SharedKey") != sharedKey) {
			return r.WriteError(http.StatusUnauthorized, nil, nil, "SharedKeyHeaderRequired", "SharedKey header required")
		}
		return r.Next(ctx)
	}
}

func NewThrottlingStage(maxRequestsPerSecond int) svrcore.Stage {
	requestPerSecond := newRateCounter(time.Second)
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if requestPerSecond.Rate() >= maxRequestsPerSecond {
			return r.WriteError(http.StatusTooManyRequests, nil, nil, "TooManyRequests", "Too many requests")
		}
		return r.Next(ctx)
	}
}

func NewDistributedTracingStage() svrcore.Stage {
	return func(ctx context.Context, r *svrcore.ReqRes) bool { return r.Next(ctx) }
}

func NewMetricsStage(logger *slog.Logger) svrcore.Stage {
	requestCountPerMinute := newRateCounter(time.Minute)
	requestLatencyPerMinute := newRateCounter(time.Minute)
	requestServiceFailuresPerMinute := newRateCounter(time.Minute)
	lastUpdate := time.Now()

	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		// Add support for https://shopify.engineering/building-resilient-payment-systems (See "4. Add Monitoring and Alerting")
		// Google’s site reliability engineering (SRE) book lists four golden signals a user-facing system should be monitored for:
		requestCountPerMinute.Add(1) // Traffic: the rate in which new work comes into the system, typically expressed in requests per minute.
		start := time.Now()
		defer func() {
			duration := time.Since(start) // Latency: the amount of time it takes to process a unit of work, broken down between success and failures.
			requestLatencyPerMinute.Add(int(duration.Milliseconds()))
			if r.RW.StatusCode >= 500 && r.RW.StatusCode < 600 {
				requestServiceFailuresPerMinute.Add(1) // Errors: the rate of unexpected service things (5xx) happening.
			}

			// Saturation: how much load the system is under, relative to its total capacity. This could be the amount of memory used versus available or a thread pool’s active threads versus total number of threads available, in any layer of the system.
			if time.Since(lastUpdate) > 1*time.Minute {
				lastUpdate = time.Now()
				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				logger.LogAttrs(ctx, slog.LevelInfo, "Metrics",
					slog.Int("req/min", requestCountPerMinute.Rate()),
					slog.Int("req ms/min", requestLatencyPerMinute.Rate()),
					slog.Int("5xx/min", requestServiceFailuresPerMinute.Rate()),
					slog.Int("HeapMem", int(memStats.Alloc)),
					slog.Int("GCs", int(memStats.NumGC)),
					slog.Int("Goroutines", runtime.NumGoroutine()))
			}
		}()
		return r.Next(ctx)
	}
}
