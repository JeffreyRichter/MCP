package serviceinfra

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewReqRes(t *testing.T) {
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if rr := NewReqRes(nil, r, nil); rr == nil {
		t.Fatal("NewReqRes returned nil")
	}
}

func TestUnmarshalRequestHeader(t *testing.T) {
	rh := RequestHeader{}
	err := UnmarshalHeaderToStruct(http.Header{
		"Authorization": []string{"granted"},
		"If-Match":      []string{"123"},
	}, &rh)
	if err != nil {
		t.Fatal(err)
	}
	if rh.Authorization == nil {
		t.Fatal("Expected Authorization to be set")
	}
	if actual := *rh.Authorization; actual != "granted" {
		t.Fatalf("wanted Authorization to be 'granted', got %q", actual)
	}
	if rh.IfMatch == nil {
		t.Fatal("Expected IfMatch to be set")
	}
	if actual := *rh.IfMatch; actual != ETag("123") {
		t.Fatal("Expected IfMatch to be '123'")
	}
}

func TestRequestHeaderVerifyStructFields(t *testing.T) {
	tests := []struct {
		name string
		rh   *RequestHeader
	}{
		{
			name: "zero value",
			rh:   &RequestHeader{},
		},
		{
			name: "all fields set",
			rh: &RequestHeader{
				Date:              Ptr(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)),
				Authorization:     Ptr("Bearer token123"),
				UserAgent:         Ptr("test-agent/1.0"),
				ContentLength:     Ptr(int64(100)),
				ContentType:       Ptr("application/json"),
				ContentEncoding:   Ptr("gzip"),
				IfMatch:           Ptr(ETag(`W/"123"`)),
				IfNoneMatch:       Ptr(ETag(`W/"456"`)),
				IfModifiedSince:   Ptr(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
				IfUnmodifiedSince: Ptr(time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)),
				Accept:            Ptr("application/json"),
				AcceptCharset:     Ptr("utf-8"),
				AcceptEncoding:    Ptr("gzip, deflate"),
				AcceptLanguage:    Ptr("en-US"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyStructFields(tt.rh)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidatePreconditions(t *testing.T) {
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		method         string
		headers        map[string]string
		resourceValues ResourceValues
		expectedCode   int
	}{
		// Error cases: resource doesn't support headers
		{
			name:   "resource doesn't support if-match",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsModified,
				ETag:                nil,
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:   "resource doesn't support if-none-match",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsModified,
				ETag:                nil,
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:   "resource doesn't support if-modified-since",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Modified-Since": baseTime.Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch,
				ETag:                Ptr(ETag("123")),
				LastModified:        nil,
			},
			expectedCode: http.StatusBadRequest,
		},
		{
			name:   "resource doesn't support if-unmodified-since",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Unmodified-Since": baseTime.Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch,
				ETag:                Ptr(ETag("123")),
				LastModified:        nil,
			},
			expectedCode: http.StatusBadRequest,
		},

		// If-Match tests
		{
			name:   "if-match matches precondition",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-match doesn't match precondition",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("456")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusPreconditionFailed,
		},

		// If-Match + If-None-Match tests
		{
			name:   "if-match and if-none-match match",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match":      "123",
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified, // If-None-Match takes precedence when both match
		},
		{
			name:   "if-match matches_if-none-match doesn't match_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match":      "123",
				"If-None-Match": "456",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-match matches_if-none-match doesn't match_unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-Match":      "123",
				"If-None-Match": "456",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},

		// If-Match + If-Modified-Since combination tests
		{
			name:   "if-match matches_if-modified-since_newer resource",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match":          "123",
				"If-Modified-Since": baseTime.Add(-time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-match matches_if-modified-since_older resource safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Match":          "123",
				"If-Modified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "if-match matches_if-modified-since_older resource_unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-Match":          "123",
				"If-Modified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},

		// If-Unmodified-Since tests
		{
			name:   "if-unmodified-since_ older resource",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-unmodified-since_ newer resource",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Unmodified-Since": baseTime.Add(-time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},

		// If-Unmodified-Since + If-None-Match combination tests
		{
			name:   "if-unmodified-since passes_if-none-match matches_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match":       "123",
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "if-unmodified-since passes_if-none-match matches_unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-None-Match":       "123",
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusPreconditionFailed,
		},
		{
			name:   "if-unmodified-since passes_if-none-match doesn't match",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match":       "456",
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},

		// If-Unmodified-Since + If-Modified-Since combination tests
		{
			name:   "if-unmodified-since passes_if-modified-since_newer resource",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
				"If-Modified-Since":   baseTime.Add(-time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-unmodified-since passes_if-modified-since_older resource_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Unmodified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
				"If-Modified-Since":   baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},

		// Standalone If-None-Match tests (no If-Match, no If-Unmodified-Since)
		{
			name:   "if-none-match matches_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "if-none-match matches_unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusPreconditionFailed,
		},
		{
			name:   "if-none-match doesn't match",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match": "456",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-none-match any_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-None-Match": "*",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "if-none-match any_ unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-None-Match": "*",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusPreconditionFailed,
		},

		// Standalone If-Modified-Since tests (no other headers)
		{
			name:   "if-modified-since_newer resource",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Modified-Since": baseTime.Add(-time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},
		{
			name:   "if-modified-since_older resource_safe method",
			method: http.MethodGet,
			headers: map[string]string{
				"If-Modified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "if-modified-since_older resource_unsafe method",
			method: http.MethodPost,
			headers: map[string]string{
				"If-Modified-Since": baseTime.Add(time.Hour).Format(http.TimeFormat),
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},

		// No conditional headers - success
		{
			name:    "no conditional headers",
			method:  http.MethodGet,
			headers: map[string]string{},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
		},

		// Test HEAD and OPTIONS as safe methods
		{
			name:   "head method with if-none-match match",
			method: http.MethodHead,
			headers: map[string]string{
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
		{
			name:   "options method with if-none-match match",
			method: http.MethodOptions,
			headers: map[string]string{
				"If-None-Match": "123",
			},
			resourceValues: ResourceValues{
				AllowedConditionals: AllowedConditionalsMatch | AllowedConditionalsModified,
				ETag:                Ptr(ETag("123")),
				LastModified:        Ptr(baseTime),
			},
			expectedCode: http.StatusNotModified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, "http://localhost", nil)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rw := httptest.NewRecorder()
			rr := NewReqRes(nil, req, rw)
			err = rr.ValidatePreconditions(tt.resourceValues)
			// ValidatePreconditions is responsible for the status code only in error
			// cases and when preconditions aren't met as stipulated in RFC 7232
			if tt.expectedCode == 0 {
				// this test case expects the preconditions to hold. The "correct" code is an
				// implementation detail we don't want to depend on here, but it certainly
				// shouldn't indicate a precondition failure
				switch rw.Code {
				case http.StatusNotModified, http.StatusPreconditionFailed:
					t.Errorf("expected preconditions to hold but ValidatePreconditions set status code %q", http.StatusText(rw.Code))
				}
				if err != nil {
					t.Error(err)
				}
				return
			}
			if err == nil {
				t.Error("expected an error because preconditions failed")
			}
			if rw.Code != tt.expectedCode {
				t.Errorf("expected %q, got %q", http.StatusText(tt.expectedCode), http.StatusText(rw.Code))
			}
		})
	}
}
