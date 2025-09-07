package svrcore

import (
	"encoding/json"
	"fmt"
)

// ServerError represents a standard Service HTTP error response as documented here:
// https://www.rfc-editor.org/rfc/rfc9457.html
type ServerError struct {
	StatusCode int    `json:"-"`
	ErrorCode  string `json:"code"`
	Message    string `json:"message,omitempty"`
}

func NewServerError(statusCode int, errorCode, messageFmt string, a ...any) *ServerError {
	return &ServerError{StatusCode: statusCode, ErrorCode: errorCode, Message: fmt.Sprintf(messageFmt, a...)}
}

// Error returns an ServerError in JSON for 4xx/5xx HTTP responses.
func (e *ServerError) Error() string {
	v := struct {
		Error *ServerError `json:"error"`
	}{Error: e}
	json := must(json.Marshal(v))
	return string(json)
}
