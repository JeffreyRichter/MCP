package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JeffreyRichter/internal/aids"
)

func (r ResourceLink) isContentBlock()     {}
func (e EmbeddedResource) isContentBlock() {}
func (t TextContent) isContentBlock()      {}
func (i ImageContent) isContentBlock()     {}
func (a AudioContent) isContentBlock()     {}

func (s StringSchema) isPrimitiveSchema()  {}
func (s NumberSchema) isPrimitiveSchema()  {}
func (s BooleanSchema) isPrimitiveSchema() {}
func (s EnumSchema) isPrimitiveSchema()    {}

const (
	StatusSubmitted                 Status = "submitted"
	StatusRunning                   Status = "running"
	StatusAwaitingSamplingResult    Status = "awaitingSamplingResult"
	StatusAwaitingElicitationResult Status = "awaitingElicitationResult"
	StatusSuccess                   Status = "success"
	StatusFailed                    Status = "failed"
	StatusCanceled                  Status = "canceled"
)

func (s Status) Processing() bool { return s == StatusSubmitted || s == StatusRunning }

func (s Status) Terminated() bool {
	return s == StatusSuccess || s == StatusFailed || s == StatusCanceled
}

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

func (er *ElicitationRequest) UnmarshalJSON(data []byte) error {
	// unmarshal into a temporary struct to handle the nested structure
	var temp struct {
		Message         string `json:"message"`
		RequestedSchema struct {
			Type       string         `json:"type"`
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required,omitempty"`
		} `json:"requestedSchema"`
	}

	if err := json.Unmarshal(data, &temp); aids.IsError(err) {
		return err
	}

	er.Message = temp.Message
	er.RequestedSchema.Type = temp.RequestedSchema.Type
	er.RequestedSchema.Required = temp.RequestedSchema.Required
	er.RequestedSchema.Properties = make(map[string]PrimitiveSchemaDefinition)
	for propName, propValue := range temp.RequestedSchema.Properties {
		propMap, ok := propValue.(map[string]any)
		if !ok {
			return errors.New("invalid property format for " + propName)
		}
		typeValue, ok := propMap["type"].(string)
		if !ok {
			return errors.New("missing type for property " + propName)
		}
		if enumValue, hasEnum := propMap["enum"]; hasEnum {
			var schema EnumSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if enumSlice, ok := enumValue.([]any); ok {
				schema.Enum = make([]string, len(enumSlice))
				for i, v := range enumSlice {
					if str, ok := v.(string); ok {
						schema.Enum[i] = str
					} else {
						return fmt.Errorf("invalid enum value for property %s", propName)
					}
				}
			}
			if enumNames, ok := propMap["enumNames"].([]any); ok {
				schema.EnumNames = make([]string, len(enumNames))
				for i, v := range enumNames {
					if str, ok := v.(string); ok {
						schema.EnumNames[i] = str
					}
				}
			}
			er.RequestedSchema.Properties[propName] = schema
			continue
		}
		switch typeValue {
		case "string":
			var schema StringSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if minLen, ok := propMap["minLength"].(float64); ok {
				intVal := int(minLen)
				schema.MinLength = &intVal
			}
			if maxLen, ok := propMap["maxLength"].(float64); ok {
				intVal := int(maxLen)
				schema.MaxLength = &intVal
			}
			if format, ok := propMap["format"].(string); ok {
				schema.Format = &format
			}
			er.RequestedSchema.Properties[propName] = schema

		case "number", "integer":
			var schema NumberSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if min, ok := propMap["minimum"].(float64); ok {
				schema.Minimum = &min
			}
			if max, ok := propMap["maximum"].(float64); ok {
				schema.Maximum = &max
			}
			er.RequestedSchema.Properties[propName] = schema

		case "boolean":
			var schema BooleanSchema
			schema.Type = typeValue
			if title, ok := propMap["title"].(string); ok {
				schema.Title = &title
			}
			if desc, ok := propMap["description"].(string); ok {
				schema.Description = &desc
			}
			if def, ok := propMap["default"].(bool); ok {
				schema.Default = &def
			}
			er.RequestedSchema.Properties[propName] = schema

		default:
			return fmt.Errorf("unknown primitive schema type: %s for property %s", typeValue, propName)
		}
	}

	return nil
}
