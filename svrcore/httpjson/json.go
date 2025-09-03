package httpjson

import (
	"fmt"
	"reflect"

	"github.com/JeffreyRichter/svrcore/syncmap"
)

// Holds sentinel values used to send nulls
var literalNullSentinels syncmap.Map[reflect.Type, any /* pointer to reflect.Type value */]

// LiteralNullFor returns a singleton representing a literal 'null' for any *T/string/slice/map type.
func LiteralNullFor[T any]() T {
	return LiteralNull(reflect.TypeFor[T]()).(T)
}

// LiteralNull returns a singleton representing a literal 'null' for any *T/slice/map type.
func LiteralNull(t reflect.Type) any {
	if sentinel, found := literalNullSentinels.Load(t); found {
		return sentinel
	}
	var sentinelValue reflect.Value
	switch t.Kind() {
	case reflect.Pointer:
		sentinelValue = reflect.New(t.Elem()) // New returns *T to a zero value of type referred to by t

	case reflect.Slice:
		sentinelValue = reflect.MakeSlice(t, 0, 1)

	case reflect.Map:
		sentinelValue = reflect.MakeMap(t)

	default:
		panic(fmt.Sprintf("Unsupported type: %v", t))
	}
	sentinel := sentinelValue.Interface()
	literalNullSentinels.Store(t, sentinel)
	return sentinel // return the pointer to sentinel value
}

// IsLiteralNull returns true if v refers to the singleton representing a JSON 'null' previously returned by JsonNull or JsonNullFor.
func IsLiteralNull(v any) bool {
	if v == nil { // v can't be nil
		return false
	}
	t := reflect.TypeOf(v)
	switch t.Kind() {
	case reflect.Pointer:
		if sentinel, found := literalNullSentinels.Load(t); found { // The type the pointer points to
			return v == reflect.ValueOf(sentinel).Interface()
		}

	case reflect.Slice, reflect.Map:
		if sentinel, found := literalNullSentinels.Load(t); found {
			return reflect.ValueOf(v).Pointer() == reflect.ValueOf(sentinel).Pointer()
		}
	}
	return false
}

// type jsonArray = []any
type JsonObject = map[string]any

// Algorithm is from https://www.rfc-editor.org/rfc/rfc7386#section-2
func JsonMergePatch(target any, patch any) any {
	// If patch is not an object, we replace the entire target with the patch
	if patchJSONObject, ok := patch.(JsonObject); !ok {
		return patch
	} else {
		t := target.(JsonObject)
		for name, value := range patchJSONObject {
			if IsLiteralNull(value) { // key's value == 'null'?
				delete(t, name)
			} else {
				t[name] = JsonMergePatch(t[name], value)
			}
		}
		return t
	}
}
