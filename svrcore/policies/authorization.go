package policies

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/svrcore"
)

func NewAuthorizationPolicy(key string) svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		if key != "" && (r.H.Authorization == nil || *r.H.Authorization != key) {
			return r.WriteError(http.StatusUnauthorized, nil, nil, "Unauthorized", "Authorization failed")
		}
		return r.Next(ctx)
	}
}
