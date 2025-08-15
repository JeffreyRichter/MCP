package serviceinfra

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/JeffreyRichter/serviceinfra/httpjson"
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
	err := httpjson.UnmarshalHeaderToStruct(http.Header{
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
			err := httpjson.VerifyStructFields(tt.rh)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
