package policies

import (
	"context"
	"net/http"

	"github.com/JeffreyRichter/svrcore"
)

func NewAuthorizationPolicy(key string) svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) bool {
		if key != "" && (r.R.Header.Get("SharedKey") != key) {
			return r.WriteError(http.StatusUnauthorized, nil, nil, "SharedKeyHeaderRequired", "SharedKey header required")
		}
		return r.Next(ctx)
	}
}
