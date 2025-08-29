package serviceinfra

import (
	"net/http"
	"time"
)

type AllowedConditionals int

func (ac AllowedConditionals) Check(match AllowedConditionals) bool {
	return ac&match != 0
}

// AllowedConditionals flags indicate which conditional headers are supported by a resource.
const (
	AllowedConditionalsNone  AllowedConditionals = 0
	AllowedConditionalsMatch AllowedConditionals = 1 << iota
	AllowedConditionalsModified
)

// ResourceValues are resource-specific values used to validate the request
type ResourceValues struct {
	AllowedConditionals AllowedConditionals
	ETag                *ETag
	LastModified        *time.Time
}

// Conditionals represents the conditional request headers from a client.
// An empty string indicates the header was not present.
type Conditionals struct {
	IfMatch           *ETag
	IfNoneMatch       *ETag
	IfModifiedSince   *time.Time
	IfUnmodifiedSince *time.Time
}

// ValidatePreconditions checks a resource's current ETag/LastModified values against a request's
// If(None)Match & If(Un)ModifiedSince headers. If preconditions pass, ValidatePreconditions returns nil; else,
// it returns an appropriate ServiceError (BadRequest, NotModified [for a safe method],
// PreconditionFailed [for an unsafe method]).
func ValidatePreconditions(rv ResourceValues, method string, c Conditionals) error {
	if !rv.AllowedConditionals.Check(AllowedConditionalsMatch) && (c.IfMatch != nil || c.IfNoneMatch != nil) {
		return NewServiceError(http.StatusBadRequest, "", "if-match and if-none-match headers not supported by this resource")
	}

	if !rv.AllowedConditionals.Check(AllowedConditionalsModified) && (c.IfModifiedSince != nil || c.IfUnmodifiedSince != nil) {
		return NewServiceError(http.StatusBadRequest, "", "if-modified-since and if-unmodified-since headers not supported by this resource")
	}

	// Method doesn't alter resource: https://developer.mozilla.org/en-US/docs/Glossary/Safe/HTTP
	methodIsSafe := method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
	statusCode := http.StatusPreconditionFailed
	if methodIsSafe {
		statusCode = http.StatusNotModified
	}

	// 1. Evaluate If-Match precondition. If-match must be checked before if-None-Match (RFC7232)
	if c.IfMatch != nil { // if-match failures always return 412; never 304
		if *c.IfMatch == ETagAny {
			// If "*" is used, the request fails if the resource doesn't exist.
			// Assuming the resource exists since we have an ETag, so this is a match.
			// The only way this would fail is if rv.ETag was empty.
			if rv.ETag == nil {
				return NewServiceError(http.StatusPreconditionFailed, "Resource does not exist", "")
			}
		} else {
			if rv.ETag == nil || !c.IfMatch.Equals(*rv.ETag) {
				return NewServiceError(http.StatusPreconditionFailed, "Resource etag doesn't match", "")
			}
		}
	}

	// 2. Evaluate If-Unmodified-Since (if If-Match is not present).
	// If-match is a stronger comparison than if-unmodifed-since
	if c.IfMatch == nil && c.IfUnmodifiedSince != nil && rv.LastModified != nil {
		if rv.LastModified.After(*c.IfUnmodifiedSince) {
			return NewServiceError(statusCode, "Resource was modified since", "")
		}
	}

	// 3. Evaluate If-None-Match (if If-Match and If-Unmodified-Since checks passed).
	// GET/HEAD failures should set these response headers: cache-control, etag, expires
	if c.IfNoneMatch != nil {
		if *c.IfNoneMatch == ETagAny {
			// If "*" is used, the request fails if the resource exists.
			if rv.ETag != nil {
				return NewServiceError(statusCode, "Resource exists", "")
			}
		} else {
			if rv.ETag != nil && c.IfNoneMatch.Equals(*rv.ETag) {
				return NewServiceError(statusCode, "Resource etag matches", "")
			}
		}
	}

	// 4. Evaluate If-Modified-Since (if If-None-Match is not present, for GET/HEAD/OPTIONS).
	if c.IfNoneMatch == nil && methodIsSafe && c.IfModifiedSince != nil && rv.LastModified != nil {
		if !rv.LastModified.After(*c.IfModifiedSince) {
			return NewServiceError(statusCode, "Resource not modified since", "")
		}
	}

	// If all preconditions pass or no conditional headers were provided, the request succeeds.
	return nil // http.StatusOK
}
