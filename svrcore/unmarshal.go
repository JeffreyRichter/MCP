package svrcore

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JeffreyRichter/svrcore/syncmap"
)

// QueryTo "deserializes" a URL's query parameter names/values to an instance of T.
func UnmarshalQueryToStruct(values url.Values, s any) error {
	return unmarshalMapOfSliceOfStrings(values, s)
}

// UnmarshalHeaderToStruct "deserializes" a URL's header keys/values to an instance of T.
func UnmarshalHeaderToStruct(header http.Header, s any) error {
	// Copy header to h to avoid mutating header & lowercasing all header keys
	h := map[string][]string{}
	for k, val := range header {
		h[strings.ToLower(k)] = val
	}
	return unmarshalMapOfSliceOfStrings(h, s)
}

// mapOfSliceOfStrings unmarshals Headers & QueryParameters. If T has an
// "Unknown" field of type Unknown, this functions sets it
func unmarshalMapOfSliceOfStrings(values map[string][]string, s any) error {
	uf := Unknown{}
	values = maps.Clone(values) // Don't modify passed-in values
	o := map[string]any{}
	fis := getFieldInfos(reflect.TypeOf(s))
	for k, val := range values {
		i := slices.IndexFunc(fis, func(fi fieldInfo) bool { return fi.name == k })
		if i == -1 { // Unknown field
			uf = append(uf, k)
			continue
		}

		if len(val) > 1 {
			panic("length of value slice >1")
		}
		// Convert string value to JSON type corrsponding to struct's field type
		switch fis[i].kind {
		case reflect.Bool:
			o[k] = val[0] == "true"

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Float32, reflect.Float64:
			if n, err := strconv.ParseFloat(val[0], 64); err != nil {
				return err
			} else {
				o[k] = n
			}

		case reflect.String:
			o[k] = val[0]

		case reflect.Struct:
			if !fis[i].timeFmt.isSet {
				panic(fmt.Sprintf("Struct field kind %q not supported (missing time format tag)", fis[i].structField))
			}
			if format := fis[i].timeFmt.value; format != "RFC1123" {
				panic(fmt.Sprintf("unsupported time format %q", format))
			}
			t, err := time.Parse(http.TimeFormat, val[0])
			if err != nil {
				return fmt.Errorf("failed to parse time field %q: %v", k, err)
			}
			o[k] = t

		case reflect.Slice:
			panic("slice not supported")

		default:
			panic(fmt.Sprintf("Field kind '%v' not supported", fis[i].kind))
		}
	}

	structValue := reflect.ValueOf(s).Elem()
	for k, v := range o {
		i := slices.IndexFunc(fis, func(fi fieldInfo) bool { return fi.name == k })
		if i == -1 {
			continue // This shouldn't happen since we already checked above
		}

		fieldValue := structValue.FieldByName(fis[i].structField)
		if !fieldValue.CanSet() {
			continue
		}

		if fieldValue.Type().Kind() == reflect.Pointer {
			elemType := fieldValue.Type().Elem()
			newValue := reflect.New(elemType)
			newValue.Elem().Set(reflect.ValueOf(v).Convert(elemType))
			fieldValue.Set(newValue)
		} else {
			fieldValue.Set(reflect.ValueOf(v).Convert(fieldValue.Type()))
		}
	}

	// If struct has an 'Unknown' field of type UnknownFields, set it to the unknown fields
	reflect.ValueOf(s).Elem().FieldByName("Unknown").Set(reflect.ValueOf(uf))
	return VerifyStructFields(s) // Validate the struct's fields
}

// Unknown is the type used for unknown fields after unmarshaling to a struct.
type Unknown []string

////////////////////////////////////////////////////////////////////////////////////

func VerifyStructFields(s any) error {
	if s == nil {
		return errors.New("VerifyStructFields: s cannot be nil")
	}
	if v := reflect.ValueOf(s); v.Kind() == reflect.Pointer && v.IsNil() {
		return errors.New("VerifyStructFields: s cannot be a nil pointer")
	}

	structType := dereference(reflect.TypeOf(s))
	if structType.Kind() != reflect.Struct {
		return fmt.Errorf("VerifyStructFields: s must be a struct, got %s", structType.Kind())
	}

	fieldInfos := getFieldInfos(structType)
	for fieldIndex := range fieldInfos {
		fi := fieldInfos[fieldIndex]
		fieldValue := reflect.ValueOf(s).Elem().FieldByName(fi.structField)
		if fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				// nil is a valid value for any pointer type
				continue
			}
			// dereference the pointer
			fieldValue = fieldValue.Elem()
		}

		switch fi.kind {
		case reflect.Bool:
			if !fieldValue.IsValid() || fieldValue.Kind() != reflect.Bool {
				return fmt.Errorf("field '%s' is not a bool", fi.name)
			}

		case reflect.Float32, reflect.Float64:
			if !fieldValue.IsValid() || !fieldValue.CanFloat() {
				return fmt.Errorf("field '%s' is not a number", fi.name)
			}
			if err := fi.verifyFloat(fieldValue.Float()); err != nil {
				return fmt.Errorf("field '%s' has invalid value: %v", fi.name, err)
			}

		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if !fieldValue.IsValid() || !fieldValue.CanInt() {
				return fmt.Errorf("field '%s' is not a number", fi.name)
			}
			if err := fi.verifyInt(fieldValue.Int()); err != nil {
				return fmt.Errorf("field '%s' has invalid value: %v", fi.name, err)
			}

		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if !fieldValue.IsValid() || !fieldValue.CanUint() {
				return fmt.Errorf("field '%s' is not a number", fi.name)
			}
			if err := fi.verifyUint(fieldValue.Uint()); err != nil {
				return fmt.Errorf("field '%s' has invalid value: %v", fi.name, err)
			}

		case reflect.String:
			if !fieldValue.IsValid() || fieldValue.Kind() != reflect.String {
				return fmt.Errorf("field '%s' is not a string", fi.name)
			}
			if err := fi.verifyLength(len(fieldValue.String())); err != nil {
				return fmt.Errorf("field '%s' has invalid length: %v", fi.name, err)
			}
			if fi.regx.isSet && !fi.regx.value.MatchString(fieldValue.String()) {
				return fmt.Errorf("field '%s' does not match regex: %s", fi.name, fi.regx.value.String())
			}

		case reflect.Struct:
			if err := VerifyStructFields(fieldValue.Addr().Interface()); err != nil {
				return fmt.Errorf("field %q: %v", fi.name, err)
			}

		case reflect.Slice:
			panic("slice not supported")

		default:
			panic(fmt.Sprintf("Field Kind '%s' not supported", fi.kind))
		}
	}
	return nil
}

type optional[T any] struct {
	isSet bool
	value T // only use if isSet == true
}

func newOptional[T any](value T) optional[T] {
	return optional[T]{isSet: true, value: value}
}

type fieldInfo struct {
	name           string                   // JSON field name (for all fields)
	structField    string                   // Struct field name (for validation)
	kind           reflect.Kind             // For all fields
	minval, maxval optional[float64]        // For all ints, uints, & floats
	minlen, maxlen optional[int64]          // For string, slice
	enums          optional[[]string]       // For string, []string
	regx           optional[*regexp.Regexp] // For string
	timeFmt        optional[string]         // For time.Time
}

func (fi *fieldInfo) verifyFloat(val float64) error {
	if fi.minval.isSet && (val < fi.minval.value) {
		return fmt.Errorf("%f <  %f", val, fi.minval.value)
	}
	if fi.maxval.isSet && (val > fi.maxval.value) {
		return fmt.Errorf("%f > %f", val, fi.maxval.value)
	}
	return nil
}

func (fi *fieldInfo) verifyInt(val int64) error {
	if fi.minval.isSet && (val < int64(fi.minval.value)) {
		return fmt.Errorf("%d < %d", val, int64(fi.minval.value))
	}
	if fi.maxval.isSet && (val > int64(fi.maxval.value)) {
		return fmt.Errorf("%d > %d", val, int64(fi.maxval.value))
	}
	return nil
}

func (fi *fieldInfo) verifyUint(val uint64) error {
	if fi.minval.isSet && (val < uint64(fi.minval.value)) {
		return fmt.Errorf("%d < %d", val, uint64(fi.minval.value))
	}
	if fi.maxval.isSet && (val > uint64(fi.maxval.value)) {
		return fmt.Errorf("%d > %d", val, uint64(fi.maxval.value))
	}
	return nil
}

func (fi *fieldInfo) verifyLength(length int) error {
	if fi.minlen.isSet && (length < int(fi.minlen.value)) {
		return fmt.Errorf("%d < %d", length, fi.minlen.value)
	}
	if fi.maxlen.isSet && (length > int(fi.maxlen.value)) {
		return fmt.Errorf("%d > %d", length, fi.maxlen.value)
	}
	return nil
}

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func tagTo[T any](tag reflect.StructTag, key string, convert func(val string) optional[T]) optional[T] {
	if val, ok := tag.Lookup(key); !ok {
		return optional[T]{}
	} else {
		return convert(val)
	}
}

func getFieldInfos(structType reflect.Type) []fieldInfo {
	structType = dereference(structType)
	if structType.Kind() != reflect.Struct {
		panic("getTypeInfo: structType must be a struct")
	}

	if fieldInfos, ok := typeToFieldInfos.Load(structType); ok { // Not in cache
		return fieldInfos
	}

	tagToInt64 := func(tag reflect.StructTag, key string) optional[int64] {
		return tagTo(tag, key,
			func(val string) optional[int64] {
				return newOptional(int64(must(strconv.ParseInt(val, 10, 64))))
			})
	}

	tagToFloat64 := func(tag reflect.StructTag, key string) optional[float64] {
		return tagTo(tag, key,
			func(val string) optional[float64] {
				return newOptional(float64(must(strconv.ParseFloat(val, 64))))
			})
	}

	var fieldInfos []fieldInfo
	for fieldIndex := range structType.NumField() {
		structField := structType.Field(fieldIndex)
		tag := structField.Tag

		// Get property name from 'json' tag or use field name
		propName, ok := "", false
		if propName, ok = tag.Lookup("json"); !ok {
			propName = structField.Name
		} else {
			propName = strings.Split(propName, ",")[0]
		}
		if propName == "-" {
			continue
		}

		// Slice order MUST match struct field order
		fi := fieldInfo{
			name:        propName,
			structField: structField.Name,
			kind:        dereference(structField.Type).Kind(),
			minval:      tagToFloat64(tag, "minval"),
			maxval:      tagToFloat64(tag, "maxval"),
			minlen:      tagToInt64(tag, "minlen"),
			maxlen:      tagToInt64(tag, "maxlen"),
			enums:       tagTo(tag, "enums", func(val string) optional[[]string] { return newOptional(strings.Split(val, ",")) }),
			regx: tagTo(tag, "regx",
				func(val string) optional[*regexp.Regexp] {
					return newOptional(regexp.MustCompile(val))
				}),
			timeFmt: tagTo(tag, "time", func(val string) optional[string] { return newOptional(val) }),
		}
		fieldInfos = append(fieldInfos, fi)
	}
	typeToFieldInfos.Store(structType, fieldInfos) // cache it for future use
	return fieldInfos
}

var typeToFieldInfos = syncmap.Map[reflect.Type, []fieldInfo]{}

// dereference returns the underlying type if t is a pointer type, otherwise returns t itself
func dereference(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}
