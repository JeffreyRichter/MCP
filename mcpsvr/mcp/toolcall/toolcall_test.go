package toolcall

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/mcpsvr/mcp"
)

func TestUnmarshalElicitationRequest(t *testing.T) {
	t.Run("BooleanSchema", func(t *testing.T) {
		b := []byte(`{"toolname":"pii","id":"1755566895766434","expiration":"2025-08-19T18:44:22.056185961-07:00","etag":null,"status":"awaitingElicitationResult","elicitationRequest":{"message":"approve PII","requestedSchema":{"type":"object","properties":{"approved":{"type":"boolean","title":"Approval","description":"Whether to approve PII access","default":false}},"required":["approved"]}}}`)
		tc := ToolCall{}
		err := json.Unmarshal(b, &tc)
		if aids.IsError(err) {
			t.Fatal(err)
		}

		if tc.ToolName == nil || *tc.ToolName != "pii" {
			t.Errorf("Expected ToolName to be 'pii', got %v", tc.ToolName)
		}
		if tc.Status == nil || *tc.Status != "awaitingElicitationResult" {
			t.Errorf("Expected Status to be 'awaitingElicitationResult', got %v", tc.Status)
		}
		if tc.ElicitationRequest == nil {
			t.Fatal("Expected ElicitationRequest to be populated")
		}
		if tc.ElicitationRequest.Message != "approve PII" {
			t.Errorf("Expected ElicitationRequest message to be 'approve PII', got %s", tc.ElicitationRequest.Message)
		}

		if len(tc.ElicitationRequest.RequestedSchema.Properties) != 1 {
			t.Errorf("Expected 1 property, got %d", len(tc.ElicitationRequest.RequestedSchema.Properties))
		}

		if approvedProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["approved"]; exists {
			if boolSchema, ok := approvedProp.(mcp.BooleanSchema); ok {
				if boolSchema.Type != "boolean" {
					t.Errorf("Expected boolean type, got %s", boolSchema.Type)
				}
				if boolSchema.Title == nil || *boolSchema.Title != "Approval" {
					t.Errorf("Expected title 'Approval', got %v", boolSchema.Title)
				}
				if boolSchema.Description == nil || *boolSchema.Description != "Whether to approve PII access" {
					t.Errorf("Expected description 'Whether to approve PII access', got %v", boolSchema.Description)
				}
				if boolSchema.Default == nil || *boolSchema.Default != false {
					t.Errorf("Expected default false, got %v", boolSchema.Default)
				}
			} else {
				t.Errorf("Expected BooleanSchema, got %T", approvedProp)
			}
		} else {
			t.Error("Expected 'approved' property to exist")
		}
	})

	t.Run("StringSchema", func(t *testing.T) {
		b := []byte(`{"name":"userInput","id":"1755566895766435","expiration":"2025-08-19T18:44:22.056185961-07:00","advanceQueue":"ToolCallAdvanceQueue","etag":null,"status":"awaitingElicitationResult","elicitationRequest":{"message":"Enter your name","requestedSchema":{"type":"object","properties":{"name":{"type":"string","title":"Full Name","description":"Enter your full name","minLength":2,"maxLength":100,"format":"text"},"email":{"type":"string","title":"Email Address","description":"Your email address","format":"email"}},"required":["name","email"]}}}`)
		tc := ToolCall{}
		err := json.Unmarshal(b, &tc)
		if aids.IsError(err) {
			t.Fatal(err)
		}

		if len(tc.ElicitationRequest.RequestedSchema.Properties) != 2 {
			t.Errorf("Expected 2 properties, got %d", len(tc.ElicitationRequest.RequestedSchema.Properties))
		}

		if nameProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["name"]; exists {
			if stringSchema, ok := nameProp.(mcp.StringSchema); ok {
				if stringSchema.Type != "string" {
					t.Errorf("Expected string type for name, got %s", stringSchema.Type)
				}
				if stringSchema.Title == nil || *stringSchema.Title != "Full Name" {
					t.Errorf("Expected title 'Full Name', got %v", stringSchema.Title)
				}
				if stringSchema.Description == nil || *stringSchema.Description != "Enter your full name" {
					t.Errorf("Expected description 'Enter your full name', got %v", stringSchema.Description)
				}
				if stringSchema.MinLength == nil || *stringSchema.MinLength != 2 {
					t.Errorf("Expected minLength 2, got %v", stringSchema.MinLength)
				}
				if stringSchema.MaxLength == nil || *stringSchema.MaxLength != 100 {
					t.Errorf("Expected maxLength 100, got %v", stringSchema.MaxLength)
				}
				if stringSchema.Format == nil || *stringSchema.Format != "text" {
					t.Errorf("Expected format 'text', got %v", stringSchema.Format)
				}
			} else {
				t.Errorf("Expected StringSchema for name, got %T", nameProp)
			}
		} else {
			t.Error("Expected 'name' property to exist")
		}

		if emailProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["email"]; exists {
			if stringSchema, ok := emailProp.(mcp.StringSchema); ok {
				if stringSchema.Type != "string" {
					t.Errorf("Expected string type for email, got %s", stringSchema.Type)
				}
				if stringSchema.Format == nil || *stringSchema.Format != "email" {
					t.Errorf("Expected format 'email', got %v", stringSchema.Format)
				}
			} else {
				t.Errorf("Expected StringSchema for email, got %T", emailProp)
			}
		} else {
			t.Error("Expected 'email' property to exist")
		}

		expectedRequired := []string{"name", "email"}
		if len(tc.ElicitationRequest.RequestedSchema.Required) != len(expectedRequired) {
			t.Errorf("Expected %d required fields, got %d", len(expectedRequired), len(tc.ElicitationRequest.RequestedSchema.Required))
		}
		for _, req := range expectedRequired {
			found := false
			for _, actual := range tc.ElicitationRequest.RequestedSchema.Required {
				if actual == req {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected required field '%s' not found", req)
			}
		}
	})

	t.Run("NumberSchema", func(t *testing.T) {
		b := []byte(`{"toolname":"measurement","id":"1755566895766436","expiration":"2025-08-19T18:44:22.056185961-07:00","etag":null,"status":"awaitingElicitationResult","elicitationRequest":{"message":"Enter measurements","requestedSchema":{"type":"object","properties":{"temperature":{"type":"number","title":"Temperature","description":"Temperature in Celsius","minimum":-273.15,"maximum":1000.0},"count":{"type":"integer","title":"Item Count","description":"Number of items","minimum":0,"maximum":999}},"required":["temperature"]}}}`)
		tc := ToolCall{}
		err := json.Unmarshal(b, &tc)
		if aids.IsError(err) {
			t.Fatal(err)
		}

		if len(tc.ElicitationRequest.RequestedSchema.Properties) != 2 {
			t.Errorf("Expected 2 properties, got %d", len(tc.ElicitationRequest.RequestedSchema.Properties))
		}

		if tempProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["temperature"]; exists {
			if numberSchema, ok := tempProp.(mcp.NumberSchema); ok {
				if numberSchema.Type != "number" {
					t.Errorf("Expected number type for temperature, got %s", numberSchema.Type)
				}
				if numberSchema.Title == nil || *numberSchema.Title != "Temperature" {
					t.Errorf("Expected title 'Temperature', got %v", numberSchema.Title)
				}
				if numberSchema.Description == nil || *numberSchema.Description != "Temperature in Celsius" {
					t.Errorf("Expected description 'Temperature in Celsius', got %v", numberSchema.Description)
				}
				if numberSchema.Minimum == nil || *numberSchema.Minimum != -273.15 {
					t.Errorf("Expected minimum -273.15, got %v", numberSchema.Minimum)
				}
				if numberSchema.Maximum == nil || *numberSchema.Maximum != 1000.0 {
					t.Errorf("Expected maximum 1000.0, got %v", numberSchema.Maximum)
				}
			} else {
				t.Errorf("Expected NumberSchema for temperature, got %T", tempProp)
			}
		} else {
			t.Error("Expected 'temperature' property to exist")
		}

		if countProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["count"]; exists {
			if numberSchema, ok := countProp.(mcp.NumberSchema); ok {
				if numberSchema.Type != "integer" {
					t.Errorf("Expected integer type for count, got %s", numberSchema.Type)
				}
				if numberSchema.Title == nil || *numberSchema.Title != "Item Count" {
					t.Errorf("Expected title 'Item Count', got %v", numberSchema.Title)
				}
				if numberSchema.Minimum == nil || *numberSchema.Minimum != 0 {
					t.Errorf("Expected minimum 0, got %v", numberSchema.Minimum)
				}
				if numberSchema.Maximum == nil || *numberSchema.Maximum != 999 {
					t.Errorf("Expected maximum 999, got %v", numberSchema.Maximum)
				}
			} else {
				t.Errorf("Expected NumberSchema for count, got %T", countProp)
			}
		} else {
			t.Error("Expected 'count' property to exist")
		}
	})

	t.Run("EnumSchema", func(t *testing.T) {
		b := []byte(`{"name":"selection","id":"1755566895766437","expiration":"2025-08-19T18:44:22.056185961-07:00","etag":null,"status":"awaitingElicitationResult","elicitationRequest":{"message":"Make a selection","requestedSchema":{"type":"object","properties":{"priority":{"type":"string","title":"Priority Level","description":"Select priority level","enum":["low","medium","high","critical"],"enumNames":["Low Priority","Medium Priority","High Priority","Critical Priority"]},"status":{"type":"string","title":"Status","description":"Current status","enum":["pending","approved","rejected"]}},"required":["priority"]}}}`)
		tc := ToolCall{}
		err := json.Unmarshal(b, &tc)
		if aids.IsError(err) {
			t.Fatal(err)
		}

		if len(tc.ElicitationRequest.RequestedSchema.Properties) != 2 {
			t.Errorf("Expected 2 properties, got %d", len(tc.ElicitationRequest.RequestedSchema.Properties))
		}

		if priorityProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["priority"]; exists {
			if enumSchema, ok := priorityProp.(mcp.EnumSchema); ok {
				if enumSchema.Type != "string" {
					t.Errorf("Expected string type for priority enum, got %s", enumSchema.Type)
				}
				if enumSchema.Title == nil || *enumSchema.Title != "Priority Level" {
					t.Errorf("Expected title 'Priority Level', got %v", enumSchema.Title)
				}
				expectedEnum := []string{"low", "medium", "high", "critical"}
				if len(enumSchema.Enum) != len(expectedEnum) {
					t.Errorf("Expected %d enum values, got %d", len(expectedEnum), len(enumSchema.Enum))
				}
				for i, expected := range expectedEnum {
					if i >= len(enumSchema.Enum) || enumSchema.Enum[i] != expected {
						t.Errorf("Expected enum[%d] to be '%s', got '%s'", i, expected, enumSchema.Enum[i])
					}
				}
				expectedEnumNames := []string{"Low Priority", "Medium Priority", "High Priority", "Critical Priority"}
				if len(enumSchema.EnumNames) != len(expectedEnumNames) {
					t.Errorf("Expected %d enum names, got %d", len(expectedEnumNames), len(enumSchema.EnumNames))
				}
				for i, expected := range expectedEnumNames {
					if i >= len(enumSchema.EnumNames) || enumSchema.EnumNames[i] != expected {
						t.Errorf("Expected enumNames[%d] to be '%s', got '%s'", i, expected, enumSchema.EnumNames[i])
					}
				}
			} else {
				t.Errorf("Expected EnumSchema for priority, got %T", priorityProp)
			}
		} else {
			t.Error("Expected 'priority' property to exist")
		}

		if statusProp, exists := tc.ElicitationRequest.RequestedSchema.Properties["status"]; exists {
			if enumSchema, ok := statusProp.(mcp.EnumSchema); ok {
				if enumSchema.Type != "string" {
					t.Errorf("Expected string type for status enum, got %s", enumSchema.Type)
				}
				expectedEnum := []string{"pending", "approved", "rejected"}
				if len(enumSchema.Enum) != len(expectedEnum) {
					t.Errorf("Expected %d enum values, got %d", len(expectedEnum), len(enumSchema.Enum))
				}
				for i, expected := range expectedEnum {
					if i >= len(enumSchema.Enum) || enumSchema.Enum[i] != expected {
						t.Errorf("Expected enum[%d] to be '%s', got '%s'", i, expected, enumSchema.Enum[i])
					}
				}
				// EnumNames should be empty or nil for this case
				if len(enumSchema.EnumNames) != 0 {
					t.Errorf("Expected no enum names for status, got %d", len(enumSchema.EnumNames))
				}
			} else {
				t.Errorf("Expected EnumSchema for status, got %T", statusProp)
			}
		} else {
			t.Error("Expected 'status' property to exist")
		}
	})

	t.Run("MixedSchemas", func(t *testing.T) {
		b := []byte(`{"name":"complex","id":"1755566895766438","expiration":"2025-08-19T18:44:22.056185961-07:00","etag":null,"status":"awaitingElicitationResult","elicitationRequest":{"message":"Fill out form","requestedSchema":{"type":"object","properties":{"name":{"type":"string","title":"Name","maxLength":50},"age":{"type":"integer","title":"Age","minimum":0,"maximum":150},"active":{"type":"boolean","title":"Active","default":true},"role":{"type":"string","title":"Role","enum":["admin","user","guest"]}},"required":["name","age"]}}}`)
		tc := ToolCall{}
		err := json.Unmarshal(b, &tc)
		if aids.IsError(err) {
			t.Fatal(err)
		}

		if len(tc.ElicitationRequest.RequestedSchema.Properties) != 4 {
			t.Errorf("Expected 4 properties, got %d", len(tc.ElicitationRequest.RequestedSchema.Properties))
		}

		expectedTypes := map[string]string{
			"name":   "mcp.StringSchema",
			"age":    "mcp.NumberSchema",
			"active": "mcp.BooleanSchema",
			"role":   "mcp.EnumSchema",
		}

		for propName, expectedType := range expectedTypes {
			if prop, exists := tc.ElicitationRequest.RequestedSchema.Properties[propName]; exists {
				actualType := fmt.Sprintf("%T", prop)
				if actualType != expectedType {
					t.Errorf("Expected property '%s' to be %s, got %s", propName, expectedType, actualType)
				}
			} else {
				t.Errorf("Expected property '%s' to exist", propName)
			}
		}

		expectedRequired := []string{"name", "age"}
		if len(tc.ElicitationRequest.RequestedSchema.Required) != len(expectedRequired) {
			t.Errorf("Expected %d required fields, got %d", len(expectedRequired), len(tc.ElicitationRequest.RequestedSchema.Required))
		}
	})
}
