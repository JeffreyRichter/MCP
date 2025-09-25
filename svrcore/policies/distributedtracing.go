package policies

import (
	"context"

	"github.com/JeffreyRichter/svrcore"
)

func NewDistributedTracing() svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) bool { return r.Next(ctx) }
}
