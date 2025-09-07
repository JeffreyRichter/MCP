package policies

import (
	"context"
	"log/slog"

	"github.com/JeffreyRichter/svrcore"
)

// NewRequestLogPolicy creates a new request logging policy.
func NewRequestLogPolicy(logger *slog.Logger) svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		logger.LogAttrs(ctx, slog.LevelInfo, "->", slog.Int64("id", r.ID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()))
		err := r.Next(ctx)
		logger.LogAttrs(ctx, slog.LevelInfo, "<- ", slog.Int64("id", r.ID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()),
			slog.Int("StatusCode", r.StatusCode()))
		return err
	}
}
