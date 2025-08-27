package policies

import (
	"context"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewTimeoutPolicy() serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error { return r.Next(ctx) }
}
