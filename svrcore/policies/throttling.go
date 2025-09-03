package policies

import (
	"context"
	"net/http"
	"time"

	"github.com/JeffreyRichter/svrcore"
)

func NewThrottlingPolicy(maxRequestsPerSecond int64) svrcore.Policy {
	requestPerSecond := newRateCounter(time.Second)
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		if requestPerSecond.Rate() >= int64(maxRequestsPerSecond) {
			return r.Error(http.StatusTooManyRequests, "TooManyRequests", "Too many requests")
		}
		return r.Next(ctx)
	}
}
