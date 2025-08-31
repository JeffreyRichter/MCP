package policies

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewRequestLogPolicy(logger *slog.Logger) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		lrw := &logResponseWriter{reqID: time.Now().Unix(), statusCode: http.StatusOK, ResponseWriter: r.RW}
		r.RW = lrw // Replace the ReqRes' ResponseWriter with this wrapped one
		logger.Info("-> ", slog.Int64("id", lrw.reqID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()))
		err := r.Next(ctx)
		logger.Info("<- ", slog.Int64("id", lrw.reqID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()), slog.Int("StatusCode", lrw.statusCode))
		return err
	}
}

type logResponseWriter struct {
	reqID int64 // Unique request ID (TODO: change to guid?)
	http.ResponseWriter
	statusCode int
}

func (lrw *logResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}
