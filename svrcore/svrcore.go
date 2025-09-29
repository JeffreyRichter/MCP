package svrcore

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
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
// Returning true if [ReqRes.WriteError] was called and futher processing of the HTTP request should stop.
type Policy func(context.Context, *ReqRes) bool

type ApiVersionKeyLocation int

const (
	ApiVersionKeyLocationHeader ApiVersionKeyLocation = iota
	ApiVersionKeyLocationQuery
)

type BuildHandlerConfig struct {
	// Policies is the slice of policies to execute for each request.
	Policies []Policy

	// ApiVersionKeyName is the key name used in the HTTP Request to specify the API version.
	ApiVersionKeyName string

	// ApiVersionKeyLocation specifies where to find the ApiVersionKeyName in the HTTP Request.
	ApiVersionKeyLocation ApiVersionKeyLocation

	// ApiVersionInfos is the slice of ApiVersionInfo pointers that define the supported API versions.
	ApiVersionInfos []*ApiVersionInfo

	// Logger is the logger used for logging service request processing errors.
	Logger *slog.Logger
}

// BuildHandler creates an HTTP handler that will be called for each request.
// The function returns an http.Handler usable with http.(ListenAnd)Serve(TLS).
// Error Handling: Requests come into the server and travel down through code and ultimately send a response to the client.
// To send a 2xx response, call [ReqRes.WriteSuccess] and then return nil errors up the stack.

// On the way to [ReqRes.WriteSuccess], many HTTP tests may fail; to send a 304/4xx/5xx error response, call [ReqRes.WriteError].
// [ReqRes.WriteError] returns a non-nil error (to a *ServiceError) which should be returned up the stack ensuring that deeper
// code doesn't ever get to call [ReqRes.WriteSuccess] (which would attempt to send 2 responses to the client).

// While traveling up the stack, try to avoid code that can produce more errors. However, if code does produce a new error,
// percolate the new error (not a *ServiceError) up the stack.

// When the request processing completes, BuildHandler's return [http.Handler] examines the processed request and logs errors if:
//   - [ReqRes.WriteSuccess] or [ReqRes.WriteServerError] were never called. Also, a 500-InternalServerError response is sent to the client.
//   - [ReqRes.WriteSuccess] or [ReqRes.WriteServerError] were called multiple times. Only the 1st call sends a response to the client.
//     On the 2nd+ calls, a non-*ServerError is returned up the stack (see next bullet).
//   - If the final error is not a *ServiceError (meaning a non-HTTP error occured somewhere while processing).
func BuildHandler(c BuildHandlerConfig) http.Handler {
	apiVersionToServeMuxPolicy := newApiVersionToServeMuxPolicy(c.ApiVersionInfos, c.ApiVersionKeyName, c.ApiVersionKeyLocation)
	policies := append(c.Policies, apiVersionToServeMuxPolicy)

	// Return the http.Handler called for each request by http.(ListenAnd)Serve(TLS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is the 1st function called when an HTTP request comes into the service
		rr, stop := newReqRes(policies, c.Logger, r, w)

		defer func() { // Guarantees logging errors during request processing & client gets a response
			stack := &strings.Builder{}
			if v := recover(); v != nil { // Panic: Capture error & stack trace
				stack.WriteString(fmt.Sprintf("Error: %v\n", v))
				aids.WriteStack(stack, aids.ParseStack(2))
				fmt.Fprint(os.Stderr, stack.String()) // Also write stack to stdout so it shows up in container logs
			}
			if stack.Len() == 0 && rr.numWriteHeaderCalls() == 1 {
				return // No panic & exactly 1 response sent to the client; all went as expected
			}
			c.Logger.LogAttrs(rr.R.Context(), slog.LevelError, "Request error", slog.String("id", rr.id),
				slog.String("method", rr.R.Method), slog.String("url", rr.R.URL.String()),
				slog.Int("numWriteHeaderCalls", rr.numWriteHeaderCalls()),
				slog.String("stack", aids.Iif(stack.Len() == 0, "(no panic)", stack.String())))

			if rr.numWriteHeaderCalls() == 0 { // Send client response if none sent due to panic or missing WriteError/WriteSuccess call
				rr.WriteError(http.StatusInternalServerError, nil, nil, "InternalServerError", "")
			}
		}()

		if !stop { // No error, start policies; defer above does any error logging & ensures a client response
			rr.Next(rr.R.Context())
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
	apiVersionInfos       apiVersionInfos
	apiVersionKeyName     string
	apiVersionKeyLocation ApiVersionKeyLocation
}

func newApiVersionToServeMuxPolicy(avis apiVersionInfos, apiVersionKeyName string, apiVersionKeyLocation ApiVersionKeyLocation) Policy {
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
				// Build & return a handler that knows how to create a new ReqRes with w, r & policies & starts policies
				// The last policy (apiversion) gets api-version's ServeMux, wraps reqRes inside a ResponseWriter and calls ServeHTTP.
				// The receiving handler unwraps RW to get ReqRes back and looks up httpHandlerToPolicy from ServeMux to invoke route policy
				pattern := method + " "
				if method == http.MethodPost {
					// Convert "POST /foo/bar:action" to "POST /foo/bar/:action" so that ServeMux pattern matching works
					pattern += strings.ReplaceAll(url, ":", "/:")
				} else {
					pattern += url
				}
				avi.serveMux.Handle(pattern, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// FIRST FUNCTION called by http.ServeMux.ServeHTTP
					s := w.(*smuggler)
					hackPostActionForServeHTTP(r, false)
					s.r.R = r // Replace old R with new 'r' which has PathValues set
					s.stop = s.r.validateRequestHeader(policyInfo.ValidHeader)
					if !s.stop {
						s.stop = policyInfo.Policy(s.ctx, s.r) // Smuggle the continue/stop flag  back to our caller
					}
				}))
			}
		}
	}
	return (&apiVersionToServeMuxPolicy{apiVersionInfos: avis, apiVersionKeyName: apiVersionKeyName, apiVersionKeyLocation: apiVersionKeyLocation}).next
}

// next (last policy) gets api-version's ServeMux, wraps reqRes inside a ResponseWriter and calls ServeHTTP.
func (p *apiVersionToServeMuxPolicy) next(ctx context.Context, r *ReqRes) bool {
	requestApiVersions := []string{}
	location := ""
	switch p.apiVersionKeyLocation {
	case ApiVersionKeyLocationHeader:
		requestApiVersions, location = r.R.Header[p.apiVersionKeyName], "header"
	case ApiVersionKeyLocationQuery:
		requestApiVersions, location = r.R.URL.Query()["api-version"], "query parameter"
	}

	requestApiVersion := ""
	avi := (*ApiVersionInfo)(nil)
	switch len(requestApiVersions) {
	case 0: // if no api-version header/query parameter specified and there exists an api-version of "", use it
		avi = p.apiVersionInfos.find("")
		if avi == nil {
			return r.WriteError(http.StatusBadRequest, nil, nil, "MissingApiVersionParameter", "Missing %s value", p.apiVersionKeyName)
		}

	case 1:
		requestApiVersion = requestApiVersions[0] // Use the api-version value specified
		avi = p.apiVersionInfos.find(requestApiVersion)

	default: // Too many (>1) version values specified
		return r.WriteError(http.StatusBadRequest, nil, nil, "The '%s' %s must specify a single value", p.apiVersionKeyName, location)
	}

	isApiVersionRetired := func(avi *ApiVersionInfo) bool { return !avi.RetireAt.IsZero() && time.Now().After(avi.RetireAt) }

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
		return r.WriteError(http.StatusBadRequest, nil, nil, "UnsupportedApiVersionValue",
			"Unsupported api-version '%s'. The supported api-versions are '%s'", requestApiVersion, supportedApiVersions)
	}

	hackPostActionForServeHTTP(r.R, true)
	_ /*muxHandler*/, pattern := avi.serveMux.Handler(r.R) // Gets api-version's ServeMux
	if pattern == "" {                                     // No pattern: method not supported at all for this URL Path
		// https://developer.mozilla.org/en-US/docs/Web/HTTP/Status/501
		// 501 is the appropriate response when the server does not recognize the request method and is incapable of supporting it for any resource.
		return r.WriteError(http.StatusMethodNotAllowed, nil, nil, "MethodNotAllowed", "Method not allowed for this api-version")
	}
	// Wrap reqRes inside a ResponseWriter and smuggle it through the ServeMux via ServeHTTP (which sets PathValues).
	s := &smuggler{ctx: ctx, r: r}
	avi.serveMux.ServeHTTP(s, r.R)
	return s.stop // Return the unsmuggled error
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
	stop                bool            // Used to smuggle the returning continue/stop flag
}

// Ptr converts a value to a pointer-to-value typically used when setting structure fields to be marshaled.
func Ptr[T any](t T) *T { return &t }
