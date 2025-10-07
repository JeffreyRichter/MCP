package toolcall

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/JeffreyRichter/internal/aids"
)

func NewServerDataEncoder(encryptionKeyHex string) *ServerDataEncoder {
	key := aids.Must(hex.DecodeString(encryptionKeyHex))
	aids.Assert(len(key) == 256/8, "encryption key must be 32 bytes for AES-256")
	return &ServerDataEncoder{
		encryptionKey: key,
		ivLength:      16,
		dataTTLMs:     (time.Minute * 5).Milliseconds(),
	}
}

type (
	ServerDataEncoder struct {
		encryptionKey []byte
		ivLength      int
		dataTTLMs     int64 // Time-to-Live (milliseconds)
	}

	secureServerData struct {
		Data      []byte `json:"data"`
		Timestamp int64  `json:"timestamp"`
		Nonce     string `json:"nonce"`
	}
)

func (sdc *ServerDataEncoder) Encode(clearData []byte) string {
	// Create cipher block
	block := aids.Must(aes.NewCipher(sdc.encryptionKey))

	// Generate IV
	iv := make([]byte, sdc.ivLength)
	aids.Must(io.ReadFull(rand.Reader, iv))

	// Generate nonce
	nonceBytes := make([]byte, 16)
	aids.Must(rand.Read(nonceBytes))

	ssd := secureServerData{
		Data:      clearData,
		Timestamp: time.Now().UnixMilli(),
		Nonce:     hex.EncodeToString(nonceBytes),
	}

	// Marshal ssd to JSON
	payload := aids.MustMarshal(ssd)

	// Apply PKCS7 padding
	paddedPayload := pkcs7Pad(payload, aes.BlockSize)

	// Encrypt
	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(paddedPayload))
	mode.CryptBlocks(encrypted, paddedPayload)

	// Concatenate IV + encrypted data
	result := append(iv, encrypted...)
	return base64.StdEncoding.EncodeToString(result)
}

// PKCS7 padding functions
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padText := make([]byte, padding)
	for i := range padText {
		padText[i] = byte(padding)
	}
	return append(data, padText...)
}

func (sdc *ServerDataEncoder) Decode(cipherText string) ([]byte, error) {
	// Decode base64
	buffer, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}
	if len(buffer) < sdc.ivLength {
		return nil, errors.New("invalid data: too short")
	}

	// Extract IV and encrypted data
	iv := buffer[:sdc.ivLength]
	encrypted := buffer[sdc.ivLength:]

	// Create cipher block
	block := aids.Must(aes.NewCipher(sdc.encryptionKey))

	// Decrypt
	if len(encrypted)%aes.BlockSize != 0 {
		return nil, errors.New("encrypted data is not a multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	// Remove PKCS7 padding
	unpadded := aids.Must(pkcs7Unpad(decrypted))

	// Unmarshal JSON
	var ssd secureServerData
	if err := json.Unmarshal(unpadded, &ssd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal context: %w", err)
	}

	// Validate timestamp
	if time.Now().UnixMilli()-ssd.Timestamp > sdc.dataTTLMs {
		return nil, errors.New("data expired")
	}
	return ssd.Data, nil
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("data is empty")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding > aes.BlockSize {
		return nil, errors.New("invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding")
		}
	}
	return data[:len(data)-padding], nil
}

/*
type serverDataConverter struct {
	secretKey []byte
}

func (sdc *serverDataConverter) Encode(data []byte) *string {
	if data == nil {
		return nil
	}
	h := hmac.New(sha256.New, sdc.secretKey) // Compute HMAC for data
	h.Write(data)
	mac := h.Sum(nil)
	data = append(mac, data...)                              // Prepend HMAC to data
	return aids.New(base64.StdEncoding.EncodeToString(data)) // Encode to base-64 string
}

func (sdc *serverDataConverter) Decode(serverData *string) (*Resource, error) {
	if serverData == nil {
		return nil, fmt.Errorf("serverData is nil")
	}
	data, err := base64.StdEncoding.DecodeString(*serverData) // Decode from base-64 string
	if aids.IsError(err) {
		return nil, err
	}
	// Split HMAC from data
	h := hmac.New(sha256.New, sdc.secretKey)
	if len(data) < h.Size() {
		return nil, fmt.Errorf("serverData must be at least %v bytes (after base-64 decoding)", h.Size())
	}
	h.Write(data[h.Size():]) // Compute HMAC for data portion
	mac := h.Sum(nil)
	if !hmac.Equal(data[:h.Size()], mac) { // Compare HMACs
		return nil, fmt.Errorf("serverData integrity check failed (after base-64 decoding)")
	}
	var resource Resource
	if err := json.Unmarshal(data[h.Size():], &resource); err != nil {
		return nil, err
	}
	return &resource, nil
}
*/
