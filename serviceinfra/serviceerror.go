package serviceinfra

import (
	"encoding/json"
	"fmt"
)

// ServiceError represents a standard Service HTTP error response as documented here:
// https://www.rfc-editor.org/rfc/rfc9457.html
type ServiceError struct {
	StatusCode int    `json:"-"`
	ErrorCode  string `json:"code"`
	Message    string `json:"message,omitempty"`
	Target     string `json:"target,omitempty"`
}

func NewServiceError(statusCode int, errorCode, messageFmt string, a ...any) *ServiceError {
	return &ServiceError{
		StatusCode: statusCode,
		ErrorCode:  errorCode,
		Message:    fmt.Sprintf(messageFmt, a...),
	}
}

// Error returns an ServiceError in JSON typically returned in an HTTP response.
func (e *ServiceError) Error() string {
	v := struct {
		Error *ServiceError `json:"error"`
	}{Error: e}
	json, _ := json.Marshal(v)
	return string(json)
}
