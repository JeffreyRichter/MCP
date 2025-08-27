package main

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/base64"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func validateMcpProtocolVersion(w http.ResponseWriter, r *http.Request) {
	// Validate the MCP Protocol Version from the request header
	mcpProtocolVersion := r.Header.Get("MCP-Protocol-Version")
	if mcpProtocolVersion != "2025-06-18" {
		// Respond with an error if the version is not supported
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, "Unsupported MCP Protocol Version", http.StatusBadRequest)
	}
}

func mcpServer(cmdLine string) {
	// https://modelcontextprotocol.io/specification/2025-06-18/basic/transports
	fmt.Println("Starting MCP Server instance with command line:", cmdLine)
	fs := flag.NewFlagSet("mcpServer", flag.ExitOnError)
	port := fs.Int("port", 0, "MCP Server instance listening port")
	certificatePemBase64 := fs.String("certificatePemBase64", "", "MCP Server instance Certificate PEM")
	privateKeyPemBase64 := fs.String("privateKeyPemBase64", "", "MCP Server instance Private Key PEM")

	if err := fs.Parse(strings.Fields(cmdLine)); err != nil {
		fmt.Println("Error parsing command line:", err)
		return
	}

	certificatePem := must(base64.StdEncoding.DecodeString(*certificatePemBase64))
	privateKeyPem := must(base64.StdEncoding.DecodeString(*privateKeyPemBase64))
	serverCertificate := must(tls.X509KeyPair(certificatePem, privateKeyPem))
	clientCertificates := x509.NewCertPool()
	clientCertificates.AppendCertsFromPEM(certificatePem) // The CA certificate is the localhost certificate
	mux := http.NewServeMux()
	server := &http.Server{
		Addr:    net.JoinHostPort("localhost", strconv.Itoa(*port)), // TODO: Untested
		Handler: mux,                                                // nil=DefaultServeMux
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{serverCertificate},
			ClientAuth:   tls.RequireAndVerifyClientCert, // Enable Mutual TLS (mTLS)
			ClientCAs:    clientCertificates,
		},
	}

	mux.Handle("GET /mcp/.well-known/oauth-protected-resource", http.FileServer(http.FS(content)))

	mux.HandleFunc("GET /mcp", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Jeff2")
		//fmt.Printf("Origin=" + r.Header.Get("Origin") + "\n")
		validateMcpProtocolVersion(w, r)
		sessionID := r.Header.Get("Mcp-Session-Id")
		_ = sessionID
		fmt.Fprintf(w, "This is the GET response")
	})

	mux.HandleFunc("POST /mcp", func(w http.ResponseWriter, r *http.Request) {
		validateMcpProtocolVersion(w, r)
		//w.Header().Set("Mcp-Session-Id", *sessionID)
		fmt.Fprintf(w, "This is the POST response.")
	})

	mux.HandleFunc("DELETE /mcp", func(w http.ResponseWriter, r *http.Request) {
		validateMcpProtocolVersion(w, r)
		sessionID := r.Header.Get("Mcp-Session-Id")
		_ = sessionID
		// Delete the session
		fmt.Fprintf(w, "This is the DELETE response")
	})

	time.Sleep(3 * time.Second) // Testing: delay MCP Server instance listening
	if false {                  // If Host sends port
		fmt.Printf("***** MCP Server instance listening: port=%d\n", *port)
		panic(server.ListenAndServeTLS("", ""))
	} else { // If server makes port & sends to host via stdout
		listener := must(net.Listen("tcp", "localhost:0"))
		port := listener.Addr().(*net.TCPAddr).Port
		// TODO: Send port to Host
		_ = port
		defer listener.Close()
		// Apply middlewareThree and middlewareFour to GET /foo
		//	mux.Handle("GET /foo", middlewareThree(middlewareFour(http.HandlerFunc(fooHandler))))
		server.Handler = NewCheckHeaderValueMiddleware(server.Handler, "Foo-Header", "FooValue")
		panic(server.ServeTLS(listener, "", ""))
	}
}

func NewCheckHeaderValueMiddleware(next http.Handler, header, value string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(header) != value {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// https://pkg.go.dev/embed

//go:embed index.html
var content embed.FS
