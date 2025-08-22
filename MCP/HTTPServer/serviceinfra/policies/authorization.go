package policies

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewAuthorizationPolicy(key string) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if key != "" && (r.H.Authorization == nil || *r.H.Authorization != key) {
			return r.Error(http.StatusUnauthorized, "Unauthorized", "Authorization failed")
		}
		return r.Next(ctx)
	}
}
