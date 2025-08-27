package serviceinfra

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/JeffreyRichter/serviceinfra/httpjson"
)

// ApiVersionInfo represents information about an API version.
type ApiVersionInfo struct {
	ApiVersion     string
	BaseApiVersion string
	RetireAt       time.Time
	GetRoutes      func(baseApiVersionRoutes ApiVersionRoutes) ApiVersionRoutes
	routes         ApiVersionRoutes
	serveMux       *http.ServeMux
}

// ApiVersionRoutes is a map that represents the routes for different API versions.
// The keys of the map are the URLs, and the values are maps that associate HTTP methods with MethodInfo objects.
type ApiVersionRoutes map[ /*url*/ string]map[ /*http method*/ string]*MethodInfo

// MethodInfo represents information about a method.
type MethodInfo struct {
	Policy      Policy       // Policy represents the policy associated with the method.
	ValidHeader *ValidHeader // ValidHeader represents the valid header associated with the method, if any.
}

// Policy specifies the function signature for a policy.
type Policy func(context.Context, *ReqRes) error

// BuildHandler creates an HTTP handler that will be called for each request.
// It takes in a slice of policies and a slice of ApiVersionInfo pointers.
// The function returns an http.Handler usable with http.(ListenAnd)Serve(TLS).
func BuildHandler(policies []Policy, avis []*ApiVersionInfo, maxOperationDuration time.Duration) http.Handler {
	apiVersionToServeMuxPolicy := newApiVersionToServeMuxPolicy(avis)
	policies = append(policies, apiVersionToServeMuxPolicy)

	// Return the http.Handler that will be called for each request by http.(ListenAnd)Serve(TLS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is the 1st function called when an HTTP request comes into the service
		reqRes := NewReqRes(policies, r, w)
		ctx, cancel := context.WithTimeout(reqRes.R.Context(), maxOperationDuration)
		defer cancel()
		if err := reqRes.Next(ctx); err != nil {
			fmt.Printf("Error processing request: %v\n", err)
			if err, ok := err.(*ServiceError); !ok { // A non-AzureError occured
				reqRes.Error(http.StatusInternalServerError, "InternalServerError", "")
				panic(err)
			}
		}
	})
}

type apiVersionInfos []*ApiVersionInfo

func (avi apiVersionInfos) find(apiVersion string) *ApiVersionInfo {
	n, ok := slices.BinarySearchFunc(avi, apiVersion,
		func(avi *ApiVersionInfo, j string) int { return strings.Compare(avi.ApiVersion, j) })
	if !ok {
		return nil
	}
	return avi[n]
}

type apiVersionToServeMuxPolicy struct {
	apiVersionInfos apiVersionInfos
}

func newApiVersionToServeMuxPolicy(avis apiVersionInfos) Policy {
	// Sort the ApiVersionInfos by api-version in ascending order so we can use BinarySearch later
	slices.SortFunc(avis, func(i, j *ApiVersionInfo) int { return strings.Compare(i.ApiVersion, j.ApiVersion) })

	for _, avi := range avis { // Create a ServeMux for each ApiVersionInfo (api-version)
		// If this api-version specifies a base api-version (which must be an earlier version/date),
		// find it, clone its routes and pass it to the GetRoutes function
		baseApiVersionRoutes := ApiVersionRoutes{}
		if avi.BaseApiVersion != "" { // A base api-version was specified
			baseavi := avis.find(avi.BaseApiVersion) // find it
			if baseavi == nil {
				panic(fmt.Sprintf("ApiVersion '%s' specifies non-existent BaseApiVersion '%s'",
					avi.ApiVersion, avi.BaseApiVersion))
			}
			baseApiVersionRoutes = maps.Clone(baseavi.routes) // Clone the outer map & inner maps so new version CAN modify it
			for k, v := range baseApiVersionRoutes {
				baseApiVersionRoutes[k] = maps.Clone(v)
			}
		}
		avi.routes = avi.GetRoutes(baseApiVersionRoutes) // Get this api-version's routes passing in the base routes

		avi.serveMux = http.NewServeMux() // Build the http.ServeMux for this api-version's routes
		for url, methodAndPolicyInfo := range avi.routes {
			for method, policyInfo := range methodAndPolicyInfo {
				// Build return a handler that knows how to create a new ReqRes with w, r & policies & starts policies
				// The last policy (apiversion) gets api-version's ServeMux, wraps reqRes inside a ResponseWriter and calls ServeHTTP.
				// Receiving handler unwraps RW to get ReqRes back and looks up httpHandlerToPolicy from ServeMux to invoke route policy
				pattern := method + " "
				if method == http.MethodPost {
					// Convert "POST /foo/bar:action" to "POST /foo/bar/:action" so that ServeMux pattern matching works
					pattern += strings.ReplaceAll(url, ":", "/:")
				} else {
					pattern += url
				}
				avi.serveMux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// This function is called by http.ServeMux.ServeHTTP
					s := w.(*smuggler)
					hackPostActionForServeHTTP(r, false)
					s.r.R = r // Replace old R with new 'r' which has PathValues set

					s.err = s.r.ValidateHeader(policyInfo.ValidHeader)
					if s.err == nil {
						s.err = policyInfo.Policy(s.ctx, s.r) // Smuggle any error back to our caller
					}
				}))
			}
		}
	}
	return (&apiVersionToServeMuxPolicy{apiVersionInfos: avis}).Next
}

// Next (last policy) gets api-version's ServeMux, wraps reqRes inside a ResponseWriter and calls ServeHTTP.
func (p *apiVersionToServeMuxPolicy) Next(ctx context.Context, r *ReqRes) error {
	requestApiVersions := r.R.URL.Query()["api-version"]
	if len(requestApiVersions) > 1 {
		return r.Error(http.StatusBadRequest, "URL 'api-version' query parameter must specify a single value", "")
	}

	var avi *ApiVersionInfo
	switch len(requestApiVersions) {
	case 0: // if no api-version query parameter specified and there exists an api-version of "", then use it
		avi = p.apiVersionInfos.find("")
		if avi == nil {
			return r.Error(http.StatusBadRequest, "MissingApiVersionParameter", "URL requires the 'api-version' query parameter")
		}
	case 1: // We have an api-version query parameter
		avi = p.apiVersionInfos.find(requestApiVersions[0])
	}

	isApiVersionRetired := func(avi *ApiVersionInfo) bool {
		return !avi.RetireAt.IsZero() && time.Now().After(avi.RetireAt)
	}

	if avi == nil || isApiVersionRetired(avi) || avi.serveMux == nil { // api-version not supported
		supportedApiVersions := "" // Build in reverse order; only show latest preview if after latest version
		for i := len(p.apiVersionInfos) - 1; i >= 0; i-- {
			if p.apiVersionInfos[i].ApiVersion == "" || isApiVersionRetired(p.apiVersionInfos[i]) {
				continue // Skip the special "" api-version and any retired api-versions
			}
			if i == len(p.apiVersionInfos)-1 && strings.HasSuffix(p.apiVersionInfos[i].ApiVersion, "-preview") {
				supportedApiVersions += p.apiVersionInfos[i].ApiVersion
			} else {
				if len(supportedApiVersions) > 0 {
					supportedApiVersions += ", " // If not empty, append a comma and space
				}
				supportedApiVersions += p.apiVersionInfos[i].ApiVersion
			}
		}
		return r.Error(http.StatusBadRequest, "UnsupportedApiVersionValue",
			"Unsupported api-version '%s'. The supported api-versions are '%s'", requestApiVersions[0], supportedApiVersions)
	}

	hackPostActionForServeHTTP(r.R, true)
	_ /*muxHandler*/, pattern := avi.serveMux.Handler(r.R) // Gets api-version's ServeMux
	if pattern == "" {                                     // No pattern: method not supported at all for this URL Path
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/501
		// 501 is the appropriate response when the server does not recognize the request method and is incapable of supporting it for any resource.
		return r.Error(http.StatusMethodNotAllowed, "MethodNotAllowed", "Method not allowed for this api-version")
	}
	// Wrap reqRes inside a ResponseWriter and smuggle it through the ServeMux via ServeHTTP (which sets PathValues)a.
	s := &smuggler{ctx: ctx, r: r}
	avi.serveMux.ServeHTTP(s, r.R)
	return s.err // Return the unsmuggled error
}

func hackPostActionForServeHTTP(r *http.Request, forServeHTTP bool) {
	if r.Method != http.MethodPost {
		return // If not a POST, nothing to do
	}
	if forServeHTTP {
		// Convert "POST /foo/bar:action" to "POST /foo/bar/:action" to ServeMux pattern matching works
		r.URL.Path = strings.ReplaceAll(r.URL.Path, ":", "/:")
	} else {
		// Convert "POST /foo/bar/:action" back to "POST /foo/bar:action" after ServeMux pattern matching
		r.URL.Path = strings.ReplaceAll(r.URL.Path, "/:", ":")
	}
}

type smuggler struct {
	http.ResponseWriter                 // Makes a smuggler an http.ResponseWriter so we can pass it to ServeMux.ServeHTTP (which sets PathValues)
	ctx                 context.Context // Used to smuggle the passed-in ReqRes
	r                   *ReqRes         // Used to smuggle the passed-in ReqRes
	err                 error           // Used to smuggle the returning error
}

// Ptr converts a value to a pointer-to-value typically used when setting structure fields to be marshaled.
func Ptr[T any](t T) *T { return &t }

// NOTE: The following types/functions are exported from tostruct so that most packages don't need to import it

// Unknown is the type used for unknown fields after unmarshaling to a struct.
type Unknown = httpjson.Unknown
