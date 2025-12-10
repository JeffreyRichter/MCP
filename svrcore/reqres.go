package svrcore

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	stagescore "github.com/JeffreyRichter/internal/stages"
)

// ReqRes encapsulates the incoming http.Requests and the outgoing http.ResponseWriter and is passed through the set of stages.
type ReqRes struct {
	// id is a unique ID for this ReqRes (useful for logging, etc.)
	id string

	// R identifies the incoming HTTP request
	R *http.Request

	// H identifies the deserialized standard HTTP headers
	H *RequestHeader

	// RW is the http.ResponseWriter used to write the HTTP response; it implements io.Writer.
	// Prefer using [ReqRes.WriteError], [ReqRes.WriteServerError], or [ReqRes.WriteSuccess] instead of using RW directly.
	RW *responseWriter

	// s is the slice of stages to execute for this request
	s stagescore.Stages[*ReqRes, bool]

	// l is the logger for anything related to processing the request & its response
	l *slog.Logger

	_ struct{} // Forces use of field names in composite literals
}

// responseWriter is a custom http.responseWriter that captures the status code.
type responseWriter struct {
	http.ResponseWriter
	StatusCode          int
	numWriteHeaderCalls int // When done request processing, this must be 1 or an error occurred
	rr                  *ReqRes
	_                   struct{} // Forces use of field names in composite literals
}

// WriteHeader overwrites http.ResponseWriter's WriteHeader method in order to capture the status code.
func (rww *responseWriter) WriteHeader(statusCode int) {
	rww.StatusCode = statusCode
	rww.numWriteHeaderCalls++
	rww.ResponseWriter.WriteHeader(statusCode)
	rr := rww.rr
	rr.l.LogAttrs(rr.R.Context(), slog.LevelInfo, "<-", slog.String("id", rr.id),
		slog.String("method", rr.R.Method), slog.String("url", rr.R.URL.String()),
		slog.Int("StatusCode", rww.StatusCode))
}

// newReqRes creates a new ReqRes with the specified stages, http.Request, & http.ResponseWriter.
func newReqRes(s []Stage, l *slog.Logger, r *http.Request, rw http.ResponseWriter) (*ReqRes, bool) {
	rr := &ReqRes{
		id: strconv.FormatInt(time.Now().Unix(), 10),
		s:  (stagescore.Stages[*ReqRes, bool])(s).Copy(),
		l:  l,
		R:  r,
		H:  &RequestHeader{},
		RW: &responseWriter{ResponseWriter: rw},
	}
	rr.RW.rr = rr
	rw.Header().Set("Server-Request-Id", rr.id) // Set this header now guaranteeing its return to the client

	rr.l.LogAttrs(rr.R.Context(), slog.LevelInfo, "->", slog.String("id", rr.id),
		slog.String("method", rr.R.Method), slog.String("url", rr.R.URL.String()))

	if err := unmarshalHeaderToStruct(r.Header, rr.H); aids.IsError(err) { // Deserialize standard HTTP request headers into this struct
		return nil, rr.WriteError(http.StatusBadRequest, nil, nil, "UnparsableHeaders", "The request has some invalid headers: %s", err.Error())
	}
	return rr, false
}

// Next sends the ReqRes to the next stage.
func (r *ReqRes) Next(ctx context.Context) bool { return r.s.Next(ctx, r) }

// WriteError sets the HTTP response to the specified HTTP status code, response headers, custom headers
// (a struct with fields/values or nil), errorCode, and message. rh and customHeader must be pointer-to-structures
// which contain only the following field types:
// *string, *int, *int8, *int16, *int32, *int64, *float32, *float64, *time.Time, *svrcore.ETag, []string
// WriteError logs any write errors and returns a ServerError.
// Callers should ensure no further writes are done to the ReqRes or its RW.
// For more control over a response, use ReqRes's RW (ResponseWriter) field directly instead of this method.
func (r *ReqRes) WriteError(statusCode int, rh *ResponseHeader, customHeader any, errorCode, messageFmt string, a ...any) bool {
	return r.WriteServerError(NewServerError(statusCode, errorCode, messageFmt, a...), rh, customHeader)
}

// WriteServerError sets the HTTP response to the specified ServerError (with StatusCode), response headers, and custom headers
// (a struct with fields/values or nil). rh and customHeader must be pointer-to-structures which contain only the
// following field types:
// *string, *int, *int8, *int16, *int32, *int64, *float32, *float64, *time.Time, *svrcore.ETag, []string
// WriteServerError logs any write errors and returns the passed-in ServerError (for convenience).
// Callers should ensure no further writes are done to the ReqRes or its RW.
// For more control over a response, use ReqRes's RW (ResponseWriter) field directly instead of this method.
func (r *ReqRes) WriteServerError(se *ServerError, rh *ResponseHeader, customHeader any) bool {
	// Azure only: rh.XMSErrorCode = &se.ErrorCode
	r.WriteSuccess(se.StatusCode, rh, customHeader, se.Error())
	return true
}

// WriteSuccess completes an HTTP response using the passed-in statusCode, response headers, customer headers (a struct
// with fields/values or nil), and bodyStruct marshaled to JSON (if not nil).
// rh and customHeader must be pointer-to-structures which contain only the following field types:
// *string, *int, *int8, *int16, *int32, *int64, *float32, *float64, *time.Time, *svrcore.ETag, []string
// If an error occurs, WriteSuccess logs it and always returns nil (for convenience).
// For more control over the response, use ReqRes's RW (ResponseWriter) field directly instead of this method.
func (r *ReqRes) WriteSuccess(statusCode int, rh *ResponseHeader, customHeader any, bodyStruct any) bool { // customHeader must be *struct
	if rh == nil {
		rh = &ResponseHeader{}
	}
	body, err := []byte{}, error(nil)
	if bodyStruct != nil {
		body = aids.MustMarshal(bodyStruct)
		// If bodyStruct passed, automatically set these response headers
		rh.ContentLength, rh.ContentType = aids.New(len(body)), aids.New("application/json")
	}
	fields2Header := func(rwh http.Header, ptrToStruct any) {
		if ptrToStruct == nil {
			return // Skip if nil
		}
		v := reflect.ValueOf(ptrToStruct).Elem() // Dereference *struct to get struct value
		// Fields can be *string, *int, *int8, *int16, *int32, *int64, *float32, *float64, *time.Time, *svrcore.ETag, []string
		for i := range v.NumField() { // Iterate over the struct's fields
			f := v.Field(i)
			jsonFieldName := strings.Split(reflect.TypeOf(ptrToStruct).Elem().Field(i).Tag.Get("json"), ",")[0]
			if jsonFieldName == "-" {
				continue // Skip fields with json:"-"
			}
			switch f.Kind() { // Field type kind
			case reflect.Pointer:
				if f.IsNil() {
					continue // Skip *fields with nil values
				}
				switch f = f.Elem(); f.Kind() { // Dereference *value to get value
				case reflect.String:
					rwh.Set(jsonFieldName, f.String())
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					rwh.Set(jsonFieldName, strconv.Itoa(int(f.Int())))
				case reflect.Float32, reflect.Float64:
					rwh.Set(jsonFieldName, strconv.FormatFloat(f.Float(), 'b', 4, 64)) // TODO:fix
				case reflect.Struct:
					switch v.Type() {
					case reflect.TypeFor[time.Time]():
						rwh.Set(jsonFieldName, f.Interface().(time.Time).Format(http.TimeFormat))
					case reflect.TypeFor[ETag]():
						rwh.Set(jsonFieldName, f.Interface().(ETag).String())
					}
				default:
					panic("unsupported field type")
				}
			case reflect.Slice:
				aids.Assert(f.Elem().Kind() == reflect.String, "unsupported slice field type; must be string")
				for _, s := range f.Interface().([]string) {
					rwh.Add(jsonFieldName, s)
				}
			default:
				panic("unsupported field type")
			}
		}
	}
	fields2Header(r.RW.Header(), rh)
	fields2Header(r.RW.Header(), customHeader)
	r.RW.WriteHeader(statusCode)
	if body != nil {
		_, err = r.RW.Write(body)
		aids.Assert(!errors.Is(err, http.ErrBodyNotAllowed), "RFC 7230, section 3.3. statusCodes 1xx/204/304 must not have a body")
	}
	return false
}

func (rr *ReqRes) numWriteHeaderCalls() int {
	return rr.RW.numWriteHeaderCalls
}

// https://en.wikipedia.org/wiki/List_of_HTTP_header_fields
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers
type RequestHeader struct { // HTTP/2 requires 'json' field names be lowercase
	Unknown        Unknown    `json:"-"` // Any unrecognized header names go here
	Date           *time.Time `json:"date" time:"RFC1123"`
	Authorization  *string    `json:"authorization"`
	UserAgent      *string    `json:"user-agent"`
	IdempotencyKey *string    `json:"idempotency-key"` // https://www.ietf.org/archive/id/draft-ietf-httpapi-idempotency-key-header-01.html

	// Message Body Information
	ContentLength   *int64  `json:"content-length"`
	ContentType     *string `json:"content-type"`
	ContentEncoding *string `json:"content-encoding"`

	// Conditionals
	IfMatch           *ETag      `json:"if-match"`
	IfNoneMatch       *ETag      `json:"if-none-match"`
	IfModifiedSince   *time.Time `json:"if-modified-since" format:"RFC1123"`
	IfUnmodifiedSince *time.Time `json:"if-unmodified-since" format:"RFC1123"`

	// Content Negotiation
	Accept         []string `json:"accept"`
	AcceptCharset  []string `json:"accept-charset"`
	AcceptEncoding []string `json:"accept-encoding"`
	AcceptLanguage []string `json:"accept-language"`
	AcceptRanges   *string  `json:"accept-ranges"`
	_              struct{} `json:"-"` // Forces use of field names in composite literals
}

type ResponseHeader struct { // HTTP/2 requires 'json' field names be lowercase
	// Versioning & Conditionals
	ETag         *ETag      `json:"etag"`
	LastModified *time.Time `json:"last-modified" format:"RFC1123"`

	// Message Body Information
	ContentLength      *int    `json:"content-length"`
	ContentType        *string `json:"content-type"`
	ContentEncoding    *string `json:"content-encoding"`
	ContentRange       *string `json:"content-range"`
	ContentDisposition *string `json:"content-disposition"`

	// Response Context
	RetryAfter *int32 `json:"retry-after"` // Seconds

	// Caching headers
	Expires *time.Time `json:"expires" time:"RFC1123"`

	/* CORS:
	AccessControlAllowCredentials *string `json:"access-control-allow-credentials"`
	AccessControlAllowHeaders     *string `json:"access-control-allow-headers"`
	AccessControlAllowMethods     *string `json:"access-control-allow-methods"`  // TODO: slice
	AccessControlAllowOrigin      *string `json:"access-control-allow-origin"`   // TODO: slice
	AccessControlExposeHeaders    *string `json:"access-control-expose-headers"` // TODO: slice
	AccessControlMaxAge           *string `json:"access-control-max-age"`        // Seconds
	//AccessControlRequestHeaders   *string `json:"access-control-request-headers"` // TODO: slice
	//AccessControlRequestMethod    *string `json:"access-control-request-method"`  // TODO: slice
	//Origin                        *string `json:"origin"`
	//TimingAllowOrigin             *string `json:"timing-allow-origin"` // TODO: slice
	*/
	// Azure-only: XMSErrorCode     *string `json:"x-ms-error-code"`
	// Azure-only: AzureDeprecating *string `json:"azure-deprecating"` // https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md#deprecating-behavior-notification
	_ struct{} `json:"-"` // Forces use of field names in composite literals
}

// ValidHeader are static values indicating the header values
// valid for a specific HTTP method used to validate the request's headers
type ValidHeader struct {
	MaxContentLength     int64    // if 0, no content allowed
	ContentTypes         []string // []string{"application/json"} or []string{"application/merge-patch+json"}
	ContentEncodings     []string
	Accept               []string
	PreconditionRequired bool
	_                    struct{} // Forces use of field names in composite literals
}

// ValidateHeader compares the RequestHeaders with ValidHeader and returns an
// ServiceError if the request is invalid; else nil
func (r *ReqRes) validateRequestHeader(vh *ValidHeader) bool {
	// https://www.loggly.com/blog/http-status-code-diagram/
	// Webpages about ignoring optional headers:
	//    https://github.com/microsoft/api-guidelines/pull/461/files
	//    https://github.com/microsoft/api-guidelines/issues/458
	if vh == nil {
		vh = &ValidHeader{}
	}

	// **** CONTENT PROCESSING
	// Content-Length CAN always be specified and, if so, must not be > MaxContentLength
	if r.H.ContentLength != nil && *r.H.ContentLength > vh.MaxContentLength {
		return r.WriteError(http.StatusRequestEntityTooLarge, nil, nil, "Content body too big", "content-length was %d but must be <= %d", *r.H.ContentLength, vh.MaxContentLength)
	}

	if vh.MaxContentLength == 0 { // No content expected
		if r.H.ContentType != nil || r.H.ContentEncoding != nil {
			return r.WriteError(http.StatusBadRequest, nil, nil, "No content headers allowed", "") // No content is allowed (except for Content-Length)
		}
	} else { // Content required
		if r.H.ContentLength == nil {
			return r.WriteError(http.StatusLengthRequired, nil, nil, "content-length header required", "")
		}
		if r.H.ContentType == nil {
			return r.WriteError(http.StatusUnsupportedMediaType, nil, nil, "content-type header required", "")
		}
		if !slices.Contains(vh.ContentTypes, *r.H.ContentType) {
			return r.WriteError(http.StatusUnsupportedMediaType, nil, nil, "Unsupported content-type", "content-type must be one of: %s", strings.Join(vh.ContentTypes, ", "))
		}
		if r.H.ContentEncoding != nil && !slices.Contains(vh.ContentEncodings, *r.H.ContentEncoding) {
			return r.WriteError(http.StatusUnsupportedMediaType, nil, nil, "Unsupported content-encoding", "content-encoding must be one of: %s", strings.Join(vh.ContentEncodings, ", "))
		}
		r.R.Body = http.MaxBytesReader(r.RW, r.R.Body, *r.H.ContentLength) // Limit reading body to Content-Length
	}

	if vh.PreconditionRequired && r.H.IfMatch == nil && r.H.IfNoneMatch == nil && r.H.IfModifiedSince == nil && r.H.IfUnmodifiedSince == nil {
		return r.WriteError(http.StatusPreconditionRequired, nil, nil, "Conditional header required", "")
	}

	// ***** ACCEPT PROCESSING
	containsAny := func(s1, s2 []string) bool {
		for _, v1 := range s1 {
			for _, v2 := range s2 {
				if v1 == v2 {
					return true
				}
			}
		}
		return false
	}
	if vh.Accept != nil && (r.H == nil || r.H.Accept == nil || !containsAny(vh.Accept, r.H.Accept)) {
		// Also check accept language, accept charset, accept encoding - in this order? If any fail, 406-Not Acceptable
		return r.WriteError(http.StatusNotAcceptable, nil, nil, "Unsupported Accept", "accept must be one of: %s", strings.Join(vh.Accept, ", "))
	}
	return false
}

// CheckPreconditions checks the passed-in ETag & LastModified (for the current resource) against the request's
// If(None)Match & If(Un)ModifiedSince headers. If preconditions pass, CheckPreconditions returns nil; else, it
// writes an appropriate ServerError to the HTTP response (BadRequest, NotModified [for a safe method],
// PreconditionFailed [for an unsafe method]) and returns the *ServerError.
func (r *ReqRes) CheckPreconditions(rv ResourceValues) bool {
	se := CheckPreconditions(rv, r.R.Method, AccessConditions{
		IfMatch:           r.H.IfMatch,
		IfNoneMatch:       r.H.IfNoneMatch,
		IfModifiedSince:   r.H.IfModifiedSince,
		IfUnmodifiedSince: r.H.IfUnmodifiedSince,
	})
	if se == nil {
		return false // Preconditions passed, don't stop processing
	}

	switch se.StatusCode {
	case http.StatusNotModified:
		r.WriteSuccess(http.StatusNotModified, &ResponseHeader{ETag: rv.ETag, LastModified: rv.LastModified}, nil, nil)

	default: //  http.StatusPreconditionFailed, http.StatusBadRequest
		r.WriteSuccess(se.StatusCode, &ResponseHeader{ETag: rv.ETag, LastModified: rv.LastModified}, nil, se)
	}
	return true // Stop processing
}

// UnmarshalQuery unmarshals the request's URL query parameters into the specified struct. If any query parameters
// are unrecognized, it writes an appropriate ServerError (BadRequest) to the HTTP response and returns the *ServerError.
func (r *ReqRes) UnmarshalQuery(s any) bool {
	values := r.R.URL.Query() // Each call to Query re-parses so we CAN mutate values
	if err := unmarshalQueryToStruct(values, s); aids.IsError(err) {
		return r.WriteError(http.StatusBadRequest, nil, nil, "Invalid query parameters", "%s", err.Error())
	}
	uf := reflect.ValueOf(s).FieldByName("Unknown").Interface().(Unknown)
	if len(uf) > 0 { // if any unrecognized query parameters, 400-BadRequest
		return r.WriteError(http.StatusBadRequest, nil, nil, "", "Unrecognized query parameters: %s", strings.Join(uf, ", "))
	}
	return false
}

// UnmarshalBody unmarshals the request's body into the specified struct. If the JSON is ill-formed, it writes
// an appropriate ServerError (BadRequest) to the HTTP response and returns the *ServerError.
func (r *ReqRes) UnmarshalBody(s any) bool {
	body, err := io.ReadAll(r.R.Body) // Ensure body is fully read
	defer r.R.Body.Close()
	if aids.IsError(err) {
		return r.WriteError(http.StatusBadRequest, nil, nil, "Unable to read full body", "%s", err.Error())
	}
	if err := json.Unmarshal(body, &s); aids.IsError(err) { // NOTE: jsonv2 errors if unrecognized fields are found
		return r.WriteError(http.StatusBadRequest, nil, nil, "Invalid JSON body", "%s", err.Error())
	}
	return false
}
