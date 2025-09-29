package policies

import (
	"context"
	"net/http"
	"time"

	"github.com/JeffreyRichter/svrcore"
)

func NewThrottlingPolicy(maxRequestsPerSecond int) svrcore.Policy {
	requestPerSecond := newRateCounter(time.Second)
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if requestPerSecond.Rate() >= maxRequestsPerSecond {
			return r.WriteError(http.StatusTooManyRequests, nil, nil, "TooManyRequests", "Too many requests")
		}
		return r.Next(ctx)
	}
}
