package policies

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewAuthenticationPolicy() serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		if false { // authorization failed
			r.Error(http.StatusUnauthorized, "Unauthorized", "Authorization failed")
		}
		return r.Next(ctx)
	}
}
