package svrcore

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"
)

type TestStruct struct {
	IsActive bool    `json:"isActive"`
	Name     string  `json:"name" minlen:"3" maxlen:"20" regx:"^[a-zA-Z0-9_]+$"`
	Price    float64 `json:"price" minval:"0.01" maxval:"9999.99"`

	OptionalAge   *int       `json:"age" minval:"0" maxval:"120"`
	OptionalDate  *time.Time `json:"optionalDate"`
	OptionalFlag  *bool      `json:"optionalFlag"`
	OptionalName  *string    `json:"optionalName" minlen:"2" maxlen:"30"`
	OptionalPrice *float64   `json:"optionalPrice" minval:"0" maxval:"1000"`

	Address Address  `json:"address"`
	Contact *Contact `json:"contact"`
}

type Address struct {
	Street  string `json:"street" minlen:"5" maxlen:"100"`
	City    string `json:"city" minlen:"2" maxlen:"50"`
	ZipCode string `json:"zipCode" regx:"^[0-9]{5}(-[0-9]{4})?$"`
}

type Contact struct {
	Email *string `json:"email" regx:"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,}$"`
	Phone *string `json:"phone" regx:"^\\+?[1-9]\\d{1,14}$"`
}

func TestJson2Struct(t *testing.T) {
	j := `{
		"name": "test_user",
		"isActive": true,
		"price": 99.99,
		"age": 25,
		"optionalName": "optional_test",
		"optionalPrice": 50.0,
		"address": {
			"street": "123 Main St",
			"city": "Anytown",
			"zipCode": "12345"
		},
		"contact": {
			"email": "test@example.com",
			"phone": "+1234567890"
		}
	}`

	var jsonObj map[string]any
	err := json.Unmarshal([]byte(j), &jsonObj)
	if err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// TODO: Implement unmarshaling from map[string]any to TestStruct
	// This would require implementing the obj[TestStruct](jsonObj) function
	// For now, we just verify the JSON unmarshaling works

	if name, ok := jsonObj["name"].(string); !ok || name != "test_user" {
		t.Errorf("Expected name to be 'test_user', got %v", jsonObj["name"])
	}
	if isActive, ok := jsonObj["isActive"].(bool); !ok || !isActive {
		t.Errorf("Expected isActive to be true, got %v", jsonObj["isActive"])
	}
	if price, ok := jsonObj["price"].(float64); !ok || price != 99.99 {
		t.Errorf("Expected price to be 99.99, got %v", jsonObj["price"])
	}
	if address, ok := jsonObj["address"].(map[string]any); !ok {
		t.Errorf("Expected address to be a map, got %v", jsonObj["address"])
	} else {
		if street, ok := address["street"].(string); !ok || street != "123 Main St" {
			t.Errorf("Expected address.street to be '123 Main St', got %v", address["street"])
		}
	}
}

func TestVerifyStructFields_BasicFields(t *testing.T) {
	tests := []struct {
		name      string
		input     TestStruct
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid struct with all required fields",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: false,
		},
		{
			name: "invalid name - too short",
			input: TestStruct{
				Name:     "ab",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'name' has invalid length",
		},
		{
			name: "invalid name - regex mismatch",
			input: TestStruct{
				Name:     "test-user!",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'name' does not match regex",
		},
		{
			name: "invalid price - too low",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    0.001,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'price' has invalid value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyStructFields(&tt.input)
			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.wantError && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message '%s' does not contain expected text '%s'", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestVerifyStructFields_PointerFields(t *testing.T) {
	validAge := 25
	invalidAge := -5
	validName := "optional_name"
	invalidName := "x"
	validPrice := 50.0
	invalidPrice := -10.0

	tests := []struct {
		name      string
		input     TestStruct
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid optional fields",
			input: TestStruct{
				Name:          "test_user",
				OptionalAge:   &validAge,
				OptionalName:  &validName,
				OptionalPrice: &validPrice,
				IsActive:      true,
				Price:         99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: false,
		},
		{
			name: "nil optional fields should be valid",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
				// All pointer fields are nil
			},
			wantError: false,
		},
		{
			name: "invalid optional age",
			input: TestStruct{
				Name:        "test_user",
				OptionalAge: &invalidAge,
				IsActive:    true,
				Price:       99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'age' has invalid value",
		},
		{
			name: "invalid optional name",
			input: TestStruct{
				Name:         "test_user",
				OptionalName: &invalidName,
				IsActive:     true,
				Price:        99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'optionalName' has invalid length",
		},
		{
			name: "invalid optional price",
			input: TestStruct{
				Name:          "test_user",
				OptionalPrice: &invalidPrice,
				IsActive:      true,
				Price:         99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  "field 'optionalPrice' has invalid value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyStructFields(&tt.input)
			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.wantError && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message '%s' does not contain expected text '%s'", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestVerifyStructFields_NestedStructs(t *testing.T) {
	validEmail := "test@example.com"
	invalidEmail := "not-an-email"
	validPhone := "+1234567890"
	invalidPhone := "abc123"

	tests := []struct {
		name      string
		input     TestStruct
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid nested struct (non-pointer)",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: false,
		},
		{
			name: "invalid nested struct - bad zipcode",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "invalid",
				},
			},
			wantError: true,
			errorMsg:  `field "address": field 'zipCode' does not match regex`,
		},
		{
			name: "invalid nested struct - street too short",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123",
					City:    "Anytown",
					ZipCode: "12345",
				},
			},
			wantError: true,
			errorMsg:  `field "address": field 'street' has invalid length`,
		},
		{
			name: "valid nested pointer struct",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
				Contact: &Contact{
					Email: &validEmail,
					Phone: &validPhone,
				},
			},
			wantError: false,
		},
		{
			name: "nil nested pointer struct should be valid",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
				Contact: nil,
			},
			wantError: false,
		},
		{
			name: "invalid nested pointer struct - bad email",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
				Contact: &Contact{
					Email: &invalidEmail,
					Phone: &validPhone,
				},
			},
			wantError: true,
			errorMsg:  `field "contact": field 'email' does not match regex`,
		},
		{
			name: "invalid nested pointer struct - bad phone",
			input: TestStruct{
				Name:     "test_user",
				IsActive: true,
				Price:    99.99,
				Address: Address{
					Street:  "123 Main St",
					City:    "Anytown",
					ZipCode: "12345",
				},
				Contact: &Contact{
					Email: &validEmail,
					Phone: &invalidPhone,
				},
			},
			wantError: true,
			errorMsg:  `field "contact": field 'phone' does not match regex`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyStructFields(&tt.input)
			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.wantError && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message '%s' does not contain expected text '%s'", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}

func TestVerifyStructFields_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantError bool
		errorMsg  string
	}{
		{
			name:      "non-struct input",
			input:     "not a struct",
			wantError: true,
			errorMsg:  "s must be a struct",
		},
		{
			name:      "nil input",
			input:     (*TestStruct)(nil),
			wantError: true,
			errorMsg:  "s cannot be a nil pointer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyStructFields(tt.input)
			if tt.wantError && err == nil {
				t.Errorf("expected error but got none")
			} else if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			} else if tt.wantError && err != nil && tt.errorMsg != "" {
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("error message '%s' does not contain expected text '%s'", err.Error(), tt.errorMsg)
				}
			}
		})
	}
}
