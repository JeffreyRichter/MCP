package policies

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewThrottlingPolicy(maxRequestsPerSecond int64) serviceinfra.Policy {
	cps := &countPerSecond{start: time.Now().Unix()}
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if cps.LatestRate() >= int64(maxRequestsPerSecond) {
			return r.Error(http.StatusTooManyRequests, "TooManyRequests", "Too many requests")
		}
		return r.Next(ctx)
	}
}

type countPerSecond struct {
	start int64 // Unix time allowing atomic update: Seconds since 1/1/1970
	count atomic.Int64
}

func (cps *countPerSecond) Add(delta int64) int64 { return cps.count.Add(delta) }

func (cps *countPerSecond) LatestRate() int64 {
	dur := max(time.Since(time.Unix(cps.start, 0)), 1*time.Second)
	return cps.count.Load() / int64(dur.Seconds())
}

func (cps *countPerSecond) Reset() {
	cps.start = time.Now().Unix()
	cps.count.Store(0)
}
