package toolcall

import (
	"fmt"
	"testing"
)

func TestServerDataEncoder(t *testing.T) {
	// Generate a 32-byte (256-bit) key for AES-256
	key := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	sdc := NewServerDataEncoder(key)
	encoded := sdc.Encode([]byte("Jeffrey Richter"))
	fmt.Println("Encoded:", encoded)

	// Decode state
	decoded, err := sdc.Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Decoded: ", decoded)
	if string(decoded) != "Jeffrey Richter" {
		t.Fatal("Decoded value does not match original")
	}
}
