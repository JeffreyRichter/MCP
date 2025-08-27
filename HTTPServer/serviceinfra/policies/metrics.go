package policies

import (
	"context"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewMetricsPolicy() serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error { return r.Next(ctx) }
}
