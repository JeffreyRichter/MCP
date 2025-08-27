package policies

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/JeffreyRichter/serviceinfra"
)

func NewLoggingPolicy(w io.Writer) serviceinfra.Policy {
	return func(ctx context.Context, r *serviceinfra.ReqRes) error {
		lrw := &logResponseWriter{reqID: time.Now().Unix(), statusCode: http.StatusOK, ResponseWriter: r.RW}
		r.RW = lrw // Replace the ReqRes' ResponseWriter with this wrapped one
		fmt.Fprintf(w, "[%d] %s %s\n", lrw.reqID, r.R.Method, r.R.URL.String())
		err := r.Next(ctx)
		fmt.Fprintf(w, "[%d] %s %s - Status: %d-%s\n", lrw.reqID, r.R.Method, r.R.URL.String(),
			lrw.statusCode, http.StatusText(lrw.statusCode))
		return err
	}
}

type logResponseWriter struct {
	reqID int64 // Unique request ID (change ot guid?)
	http.ResponseWriter
	statusCode int
}

func (lrw *logResponseWriter) WriteHeader(statusCode int) {
	lrw.statusCode = statusCode
	lrw.ResponseWriter.WriteHeader(statusCode)
}
