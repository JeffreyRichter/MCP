package aids

import (
	"encoding/json"
	"encoding/json/jsontext"
)

// IsError returns true if err is nil
func IsError(err error) bool { return err != nil }

// Iif is "inline if"
func Iif[T any](expression bool, trueVal, falseVal T) T {
	if expression {
		return trueVal
	}
	return falseVal
}

// Assert panics if condition is false
func Assert(condition bool, v any) {
	if !condition {
		panic(v)
	}
}

// AssertSuccess panics if err != nil
func AssertSuccess(err error) {
	Assert(!IsError(err), err)
}

// Must returns val if err is nil, otherwise panics with err
func Must[T any](val T, err error) T {
	Assert(!IsError(err), err)
	return val
}

func Marshal(v any) jsontext.Value { return Must(json.Marshal(v)) }

func Unmarshal[T any](data []byte) T {
	var t T
	AssertSuccess(json.Unmarshal(data, &t))
	return t
}
