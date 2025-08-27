package main

// mTLS: https://www.bastionxp.com/blog/golang-https-web-server-self-signed-ssl-tls-x509-certificate/
import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

/* Benefits of using this pattern for LOCAL MCP Servers instead of the stdio transport:
- All MCP Hosts/Servers use the same HTTP transport, which allows for consistent handling of requests and responses.
- Allows each GET request to have its own stream
- MCP Hosts/Servers use HTTP, not HTTPS
- Maintains a 1:1 relationship between MCP Host & MCP Server
- No authentication or tenant tracking required; ONE Session ID per MCP Host/Server instance supported
- The MCP Server process can crash and be restarted restoring its session (from SessionID)
*/

func main() { // This is the MCP Host code
	port, mcpClient := mcpServerInstanceStart()
	endpoint := fmt.Sprintf("https://%s/mcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
	err := mcpServerInstanceListening(mcpClient, endpoint, 50*time.Second)
	if err != nil {
		fmt.Println("MCP Server instance did not start in time:", err)
	}

	{
		resp := must(mcpClient.Get(endpoint + "/.well-known/oauth-protected-resource")) // /index.html
		defer resp.Body.Close()
		body := string(must(io.ReadAll(resp.Body)))
		fmt.Printf("index.html: %s\n\n", body)
	}

	// Call the MCP Server instance
	req := must(http.NewRequest(http.MethodPost, endpoint, nil))
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	resp, err := mcpClient.Do(req)
	if err != nil {
		panic(err) // The MCP Server request failed
	}
	defer resp.Body.Close()
	body := string(must(io.ReadAll(resp.Body)))
	fmt.Printf("MCP Server response: %s\n\n", body)

	fmt.Println("Presss <Enter> to exit.")
	fmt.Fscanln(os.Stdin)
	fmt.Println("MCP Host exiting.")
}

// mcpServerInstanceStart allows an MCP Host to start a LOCAL MCP Server & communicates via the Streamable HTTP transport
func mcpServerInstanceStart() (port int, mcpClient *http.Client) {
	// The MCP Client & Server communicate via a port on localhost using mutual TLS so that no
	// other process can talk to the MCP Server.

	// Create a self-signed certificate AND KEY for mutual TLS
	certificatePEM, privateKeyPEM := createSelfSignedCertificate(time.Now().Add(time.Hour * 24 * 365 * 10)) // 10 years

	// Find a unique localhost port for the MCP Host/Server to communicate
	listener := must(net.Listen("tcp", "localhost:0"))
	port = listener.Addr().(*net.TCPAddr).Port
	listener.Close() // Release the port so the MCP Server can listen on it

	// Start the MCP Server process (we use a goroutine for testing purposes)
	go mcpServer(
		fmt.Sprintf("-port=%d -certificatePemBase64=%s -privateKeyPemBase64=%s",
			port, base64.StdEncoding.EncodeToString(certificatePEM), base64.StdEncoding.EncodeToString(privateKeyPEM)))

	// The HTTP client accepts the MCP Server's certificate and presents its own (same) certificate
	serverCertificates := x509.NewCertPool()
	serverCertificates.AppendCertsFromPEM(certificatePEM)
	clientCertificate := must(tls.X509KeyPair(certificatePEM, privateKeyPEM))
	mcpClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
				RootCAs:            serverCertificates,                   // Server's certificate trusted by client
				Certificates:       []tls.Certificate{clientCertificate}, // Client's certificate trusted by server
			}}}

	return port, mcpClient
}

func mcpServerInstanceListening(mcpClient *http.Client, endpoint string, maxWaitTime time.Duration) error {
	// Wait for the MCP Server instance to start listening
	resp, err := (*http.Response)(nil), error(nil)
	for start := time.Now(); time.Since(start) < maxWaitTime; time.Sleep(100 * time.Millisecond) {
		req := must(http.NewRequest(http.MethodPost, endpoint, nil))
		req.Header.Set("MCP-Protocol-Version", "2025-06-18")
		if resp, err = mcpClient.Do(req); err == nil {
			break // The MCP Server instance is listening
		}
		fmt.Printf("DEBUG - MCP Server instance not listening: %s\n\n", err)
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body := string(must(io.ReadAll(resp.Body)))
	fmt.Printf("DEBUG - MCP Server response: %s\n\n", body)
	return nil
}

func createSelfSignedCertificate(expiration time.Time) (certificatePEM, privateKeyPEM []byte) {
	// Generate RSA private key
	privateKey := must(rsa.GenerateKey(rand.Reader, 2048))

	// Define certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"MCP mTLS"},
			CommonName:   "localhost",
		},
		DNSNames:    []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:   time.Now(),
		NotAfter:    expiration,

		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, // For both client & Server auth
		IsCA:                  true,                                                                       // It's a self-signed CA
		BasicConstraintsValid: true,
	}

	// Create self-signed certificate
	derBytes := must(x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey))

	// Encode certificate to PEM
	certificatePEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Encode private key to PEM
	privateKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)})

	return certificatePEM, privateKeyPEM
}

func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}
