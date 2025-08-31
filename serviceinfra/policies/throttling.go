package policies

import (
	"context"
	"net/http"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewThrottlingPolicy(maxRequestsPerSecond int64) serviceinfra.Policy {
	requestPerSecond := newRateCounter(time.Second)
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if requestPerSecond.Rate() >= int64(maxRequestsPerSecond) {
			return r.Error(http.StatusTooManyRequests, "TooManyRequests", "Too many requests")
		}
		return r.Next(ctx)
	}
}
