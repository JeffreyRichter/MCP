package serviceinfra

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JeffreyRichter/serviceinfra/httpjson"
)

// ReqRes encapsulates the incoming http.Requests and the outgoing http.ResponseWriter and is passed through the set of policies.
type ReqRes struct {
	// R identifies the incoming HTTP request
	R *http.Request
	// H identifies the deserialized standard HTTP headers
	H *RequestHeader
	// RW is the http.ResponseWriter used to write the HTTP response; it implements io.Writer
	RW http.ResponseWriter
	p  []Policy // The slice of policies to execute for this request
}

// NewReqRes creates a new ReqRes with the specified policies, http.Request, & http.ResponseWriter.
func NewReqRes(p []Policy, r *http.Request, rw http.ResponseWriter) *ReqRes {
	var h RequestHeader
	err := httpjson.UnmarshalHeaderToStruct(r.Header, &h) // Deserialize standard HTTP request header
	if err != nil {
		panic(err)
	}
	return &ReqRes{p: p, R: r, H: &h, RW: rw}
}

// Next sends the ReqRes to the next policy.
func (r *ReqRes) Next(ctx context.Context) error {
	nextPolicy := r.p[0]
	r.p = r.p[1:]
	return nextPolicy(ctx, r)
}

// ServiceError represents a standard Service HTTP error response as documented here:
// https://www.rfc-editor.org/rfc/rfc9457.html
type ServiceError struct {
	StatusCode int    `json:"-"`
	ErrorCode  string `json:"code"`
	Message    string `json:"message,omitempty"`
	Target     string `json:"target,omitempty"`
}

// Error returns an ServiceError in JSON typically returned in an HTTP response.
func (e *ServiceError) Error() string {
	v := struct {
		Error *ServiceError `json:"error"`
	}{Error: e}
	json, _ := json.Marshal(v)
	return string(json)
}

// Error sets the HTTP response to the specified HTTP status code and Service error.
// The caller should ensure no further writes are done to the http.ResponseWriter.
func (r *ReqRes) Error(statusCode int, errorCode, messageFmt string, a ...any) error {
	se := &ServiceError{
		StatusCode: statusCode,
		ErrorCode:  errorCode,
		Message:    fmt.Sprintf(messageFmt, a...),
	}
	return r.WriteResponse(&ResponseHeader{
		XMSErrorCode: &se.ErrorCode,
		ContentType:  Ptr("application/json"),
	}, nil, se.StatusCode, se)
}

func (r *ReqRes) UnmarshalQuery(s any) error {
	values := r.R.URL.Query() // Each call to Query re-parses so we CAN mutate values
	err := httpjson.UnmarshalQueryToStruct(values, s)
	if err != nil {
		return err
	}
	uf := reflect.ValueOf(s).FieldByName("Unknown").Interface().(httpjson.Unknown)
	if len(uf) > 0 { // if any unrecognized query parameters, 400-BadRequest
		return r.Error(http.StatusBadRequest, "",
			"Unrecognized query parameters: %s", strings.Join(uf, ", "))
	}
	return nil
}

// UnmarshalBody unmarshals the request's body into the specified struct. If the JSON is illformed, it writes
// an appropriate ServiceError (BadRequest) to the HTTP response and returns non-nil.
func (r *ReqRes) UnmarshalBody(s any) error {
	if err := json.UnmarshalRead(r.R.Body, &s); err != nil {
		// Inability to unmarshal the input suggests a client-side problem.
		return r.Error(http.StatusBadRequest, "Invalid JSON body", "%s", err.Error())
	}
	/*
		if len(jsonObj) > 0 { // If unknown fields remain, set struct's Unknown field & 400-BadRequest
			uf := reflect.ValueOf(s).FieldByName("Unknown").Interface().(unmarshaltostruct.Unknown)
			err = r.Error(http.StatusBadRequest, "",
				"JSON object has unknown fields: %s", strings.Join(uf, ", "))
		}
		return err
	*/
	return nil
}

// WriteResponse completes an HTTP response by setting HTTP ResponseHeader, any
// customHeader (pass nil if none), HTTP status code, and body structure (pass nil if none).
// For more control over the response, use ReqRes's RW (ResponseWriter) field directly instead of this method.
func (r *ReqRes) WriteResponse(rh *ResponseHeader, customHeader any, statusCode int, bodyStruct any) error { // customHeader must be *struct
	body, err := []byte{}, error(nil)
	if bodyStruct != nil {
		body, err = json.Marshal(bodyStruct)
		if err != nil {
			return err
		}
		// net/http will set Content-Length
		rh.ContentType = Ptr("application/json")
	}
	rwh := r.RW.Header()
	fields2Header := func(h any) {
		v := reflect.ValueOf(h).Elem()
		for i := range v.NumField() {
			f := v.Field(i)
			jsonFieldName := strings.Split(reflect.TypeOf(h).Elem().Field(i).Tag.Get("json"), ",")[0]
			switch f.Kind() {
			case reflect.Pointer:
				if f.IsNil() {
					continue // skip any nil fields
				}
				switch f.Elem().Kind() {
				case reflect.String:
					rwh[jsonFieldName] = []string{f.Elem().String()}
				case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
					rwh[jsonFieldName] = []string{strconv.Itoa(int(f.Elem().Int()))}
				case reflect.Float32, reflect.Float64:
					rwh[jsonFieldName] = []string{strconv.FormatFloat(f.Elem().Float(), 'b', 4, 64)} // TODO:fix
				case reflect.Struct:
					switch v.Type() {
					case reflect.TypeFor[time.Time]():
						rwh[jsonFieldName] = []string{f.Elem().Interface().(time.Time).Format(http.TimeFormat)}
						//case reflect.TypeFor[etag.ETag]():
						//	rwh[jsonFieldName] = []string{f.Elem().Interface().(etag.ETag).String()}
					}
				default:
					panic("unsupported field type")
				}
			case reflect.Slice:
				if f.Interface().([]string) == nil {
					continue // skip any nil slices
				}
				rwh[jsonFieldName] = v.Interface().([]string) // Assumes slice of "string"
			default:
				panic("unsupported field type")
			}
		}

	}
	if rh != nil {
		fields2Header(rh)
	}
	if customHeader != nil {
		fields2Header(customHeader)
	}
	r.RW.WriteHeader(statusCode)
	_, err = r.RW.Write(body)
	return err
}

// https://en.wikipedia.org/wiki/List_of_HTTP_header_fields
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers
type RequestHeader struct { // 'json' tag value MUST be lowercase
	Unknown       httpjson.Unknown `json:"-"` // Any unrecognized header names go here
	Date          *time.Time       `json:"date" time:"RFC1123"`
	Authorization *string          `json:"authorization"`
	UserAgent     *string          `json:"user-agent"`
	//CacheControl  *string          `json:"cache-control"`
	//Expect        *string          `json:"expect"`

	// Message Body Information
	ContentLength   *int64  `json:"content-length"`
	ContentType     *string `json:"content-type"`
	ContentEncoding *string `json:"content-encoding"`

	// Conditionals
	IfMatch     *ETag `json:"if-match"`
	IfNoneMatch *ETag `json:"if-none-match"`
	//IfRange           *ETag      `json:"if-range"`
	IfModifiedSince   *time.Time `json:"if-modified-since" time:"RFC1123"`
	IfUnmodifiedSince *time.Time `json:"if-unmodified-since" time:"RFC1123"`

	// Content Negotiation
	Accept         *string `json:"accept"`          // TODO: slice
	AcceptCharset  *string `json:"accept-charset"`  // TODO: slice
	AcceptEncoding *string `json:"accept-encoding"` // TODO: slice
	AcceptLanguage *string `json:"accept-language"` // TODO: slice

	// Range Requests
	//Range *string `json:"range"`
}

type ResponseHeader struct { // 'json' tag value MUST be lowercase
	XMSErrorCode     *string `json:"x-ms-error-code"`
	AzureDeprecating *string `json:"azure-deprecating"` // https://github.com/microsoft/api-guidelines/blob/vNext/azure/Guidelines.md#deprecating-behavior-notification

	// Message Body Information
	ContentLength   *int    `json:"content-length"`
	ContentType     *string `json:"content-type"`
	ContentEncoding *string `json:"content-encoding"`
	//ContentRange    *string `json:"content-range"`
	//ContentDisposition *string `json:"content-disposition"`

	// Versioning & Conditionals
	ETag         *ETag      `json:"etag"`
	LastModified *time.Time `json:"last-modified" time:"RFC1123"`

	// Response Context
	//Allow      *string `json:"allow"`       // TODO: slice
	RetryAfter *int32 `json:"retry-after"` // Seconds

	// Caching headers
	//Expires *time.Time `json:"expires" time:"RFC1123"`

	/* CORS
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
}

// ValidHeader are static values indicating the header values
// valid for a specific HTTP method used to validate the request's headers
type ValidHeader struct {
	MaxContentLength     int64    // if 0, no content allowed
	ContentTypes         []string // []string{"application/json"} or []string{"application/merge-patch+json"}
	ContentEncodings     []string
	Accept               []string
	PreconditionRequired bool
}

// ValidateHeader compares the RequestHeaders with ValidHeader and returns an
// ServiceError if the request is invalid; else nil
func (r *ReqRes) ValidateHeader(vh *ValidHeader) error {
	// https://www.loggly.com/blog/http-status-code-diagram/
	// Webpages about ignoring optional headers:
	//    https://github.com/microsoft/api-guidelines/pull/461/files
	//    https://github.com/microsoft/api-guidelines/issues/458
	if vh == nil {
		vh = &ValidHeader{}
	}

	// **** CONTENT PROCESSING
	// Content-Length CAN be always be specified and, if so, must not be > MaxContentLength
	if r.H.ContentLength != nil && *r.H.ContentLength > vh.MaxContentLength {
		return r.Error(http.StatusRequestEntityTooLarge, "Content body too big", "content-length was %d but must be <= %d", *r.H.ContentLength, vh.MaxContentLength)
	}

	if vh.MaxContentLength == 0 { // No content expected
		if r.H.ContentType != nil || r.H.ContentEncoding != nil {
			return r.Error(http.StatusBadRequest, "No content headers allowed", "") // No content is allowed (except for Content-Length)
		}
	} else { // Content required
		if r.H.ContentLength == nil {
			return r.Error(http.StatusLengthRequired, "content-length header required", "")
		}
		if r.H.ContentType == nil {
			return r.Error(http.StatusUnsupportedMediaType, "content-type header required", "")
		}
		if !slices.Contains(vh.ContentTypes, *r.H.ContentType) {
			return r.Error(http.StatusUnsupportedMediaType, "Unsupported content-type", "content-type must be one of: %s", strings.Join(vh.ContentTypes, ", "))
		}
		if r.H.ContentEncoding != nil && !slices.Contains(vh.ContentEncodings, *r.H.ContentEncoding) {
			return r.Error(http.StatusUnsupportedMediaType, "Unsupported content-encoding", "content-encoding must be one of: %s", strings.Join(vh.ContentEncodings, ", "))
		}
	}

	if vh.PreconditionRequired && r.H.IfMatch == nil && r.H.IfNoneMatch == nil && r.H.IfModifiedSince == nil && r.H.IfUnmodifiedSince == nil {
		return r.Error(http.StatusPreconditionRequired, "Conditional header required", "")
	}

	// ***** ACCEPT PROCESSING
	if vh.Accept != nil && (r.H == nil || r.H.Accept == nil || !slices.Contains(vh.Accept, *r.H.Accept)) {
		// Also check accept language, accept charset, accept encoding - in this order? If any fail, 406-Not Acceptable
		return r.Error(http.StatusNotAcceptable, "Unsupported Accept", "accept must be one of: %s", strings.Join(vh.Accept, ", "))
	}
	return nil
}

// PreconditionValues are resource-specific values used to validate the request
type PreconditionValues struct {
	ETag         *ETag
	LastModified *time.Time
}

// ValidatePreconditions checks the passed-in ETag & LastModified (for the current resource) against the request's
// If(None)Match & Id(Un)ModifiedSince headers. If preconditions pass, ValidatePreconditions returns nil; else, it
// writes an appropriate ServiceError to the HTTP response (BadRequest, NotModified [for a safe method],
// PreconditionFailed [for an unsafe method]) and return non-nil.
func (r *ReqRes) ValidatePreconditions(rv *PreconditionValues) error {
	if rv.ETag == nil && (r.H.IfMatch != nil || r.H.IfNoneMatch != nil) {
		return r.Error(http.StatusBadRequest, "", "if-match and if-none-match headers not supported by this resource")
	}

	if rv.LastModified == nil && (r.H.IfModifiedSince != nil || r.H.IfUnmodifiedSince != nil) {
		return r.Error(http.StatusBadRequest, "", "if-modified-since and if-unmodified-since headers not supported by this resource")
	}

	isMethodSafe := func() bool { // Method doesn't alter resource: https://developer.mozilla.org/en-US/docs/Glossary/Safe/HTTP
		return r.R.Method == http.MethodGet || r.R.Method == http.MethodHead || r.R.Method == http.MethodOptions
	}

	// NOTE ORDER: If-match must be checked before if-None-Match (RFC7232)
	// Algorithm from : https://www.loggly.com/blog/http-status-code-diagram/
	hasIfNoneMatch := func() error {
		if r.H.IfNoneMatch != nil {
			if r.H.IfNoneMatch.Equals(ETagAny) || r.H.IfNoneMatch.WeakEquals(*rv.ETag) {
				if isMethodSafe() {
					return r.Error(http.StatusNotModified, "Resource etag matches", "")
				} else {
					return r.Error(http.StatusPreconditionFailed, "Resource etag matches", "")
				}
			}
			return nil
		} else {
			// RFC 7232 3.3: if-modified-since applies only to GET and HEAD
			if r.H.IfModifiedSince != nil && (r.R.Method == http.MethodGet || r.R.Method == http.MethodHead) {
				if rv.LastModified.After(*r.H.IfModifiedSince) { // true if resource date after If-Mmodifed-Since date
					return nil
				}
				return r.Error(http.StatusNotModified, "Resource not modified since", "")
			} else {
				return nil
			}
		}
	}
	if r.H.IfMatch != nil {
		if !rv.ETag.Equals(*r.H.IfMatch) {
			return r.Error(http.StatusPreconditionFailed, "Etags do not match", "")
		} else {
			return hasIfNoneMatch()
		}
	} else {
		if r.H.IfUnmodifiedSince != nil {
			if rv.LastModified.Before(*r.H.IfUnmodifiedSince) { // true if resource date before If-Unmodifed-Since date
				return hasIfNoneMatch()
			} else {
				return r.Error(http.StatusPreconditionFailed, "Resource not modified since", "")
			}
		} else {
			return hasIfNoneMatch()
		}
	}
	/* Algorithm from: https://www.loggly.com/blog/http-status-code-diagram/
	has-if-match {
	   if-match doesn't match {
	      412
	   } else {
	      has-if-none-match {
	         if-none-match matches {
	            Success
	         } else {
	            if is-precondition-safe {
				   304
				} else {
				   412
				}
	         }
	      } else {
	         has-if-modified-since {
	            if-modified-since matches {
	                Success
	            } else {
	               if is-precondition-safe {
				      304
				   } else {
					  412
				   }
				}
	         } else {
	            Success
	         }
	      }
	   }
	} else {// !has-if-match
	   has-if-unmodified-since {
	      if-unmodified-since matches {
			goto "has if_none_match"
		  } else {
			412
		  }
	   } else {
	      goto "has_if_none_match"
	   }
	}
	*/
}
