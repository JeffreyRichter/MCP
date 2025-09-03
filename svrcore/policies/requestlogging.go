package policies

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/JeffreyRichter/svrcore"
)

// NewRequestLogPolicy creates a new request logging policy.
func NewRequestLogPolicy(logger *slog.Logger) svrcore.Policy {
	return func(ctx context.Context, r *svrcore.ReqRes) error {
		lrw := &logResponseWriter{reqID: time.Now().Unix(), statusCode: http.StatusOK, ResponseWriter: r.RW}
		r.RW = lrw // Replace the ReqRes' ResponseWriter with this wrapped one
		logger.Info("-> ", slog.Int64("id", lrw.reqID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()))
		err := r.Next(ctx)
		logger.Info("<- ", slog.Int64("id", lrw.reqID), slog.String("method", r.R.Method), slog.String("url", r.R.URL.String()), slog.Int("StatusCode", lrw.statusCode))
		return err
	}
}

// logResponseWriter is a custom http.ResponseWriter that captures a unique request ID and status code.
type logResponseWriter struct {
	reqID int64 // Unique request ID (TODO: change to guid?)
	http.ResponseWriter
	statusCode int
}

// WriteHeader overwrites http.ResponseWriter's WriteHeader method in order to capture the status code (fror logging).
func (lrw *logResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}
