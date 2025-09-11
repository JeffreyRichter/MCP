package svrcore

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JeffreyRichter/internal/aids"
	"github.com/JeffreyRichter/svrcore/syncmap"
)

// Unknown is the type used for unknown fields after unmarshaling to a struct.
type Unknown []string

// QueryTo "deserializes" a URL's query parameter names/values to an instance of T.
func unmarshalQueryToStruct(values url.Values, s any) error {
	return unmarshalMapOfSliceOfStrings(values, s)
}

// unmarshalHeaderToStruct "deserializes" an http.Header's keys/values to an instance of s (passed-by-pointer).
func unmarshalHeaderToStruct(header http.Header, s any) error {
	// Copy header to h to avoid mutating header & lowercasing all header keys
	h := map[string][]string{}
	for k, val := range header {
		h[strings.ToLower(k)] = val
	}
	return unmarshalMapOfSliceOfStrings(h, s)
}

// mapOfSliceOfStrings "deserializes" a map[string][]string to an instance of s (a *struct).
// If *s has an "Unknown" field of type Unknown ([]string), this function put unrecognized keys in it.
func unmarshalMapOfSliceOfStrings(jsonFieldNameToJsonFieldSlice map[string][]string, s any) error {
	setStructField := func(s any, fi fieldInfo, jsonFieldValue any) {
		structValue := reflect.ValueOf(s).Elem()            // Structure's *T --> T
		fieldValue := structValue.FieldByName(fi.fieldName) // Field structure's fieldName's value
		// if !fieldValue.CanSet() { return }
		if fieldValue.Type().Kind() == reflect.Pointer {
			elemType := fieldValue.Type().Elem() // Get field's type (without the pointer)
			newValue := reflect.New(elemType)    // Createa a new instance of the field's type
			newValue.Elem().Set(reflect.ValueOf(jsonFieldValue).Convert(elemType))
			fieldValue.Set(newValue) // Set the field's new value
		} else {
			fieldValue.Set(reflect.ValueOf(jsonFieldValue).Convert(fieldValue.Type()))
		}
	}

	unknownJsonFieldNames := Unknown{}
	fis := getFieldInfos(reflect.TypeOf(s))
	for inputJsonFieldName, inputJsonFieldSlice := range jsonFieldNameToJsonFieldSlice {
		// Lookup the fieldInfo for this JSON field name
		i := slices.IndexFunc(fis, func(fi fieldInfo) bool { return fi.jsonName == inputJsonFieldName })
		if i == -1 { // Unknown JSON field name
			unknownJsonFieldNames = append(unknownJsonFieldNames, inputJsonFieldName)
			continue
		}

		// Based on the structure's type for this JSON field name, convert inputJsonFieldSlice to an instance of the corresponding structure field type
		switch fis[i].fieldType {
		case reflect.TypeFor[*bool]():
			setStructField(s, fis[i], inputJsonFieldSlice[0] == "true")

		case reflect.TypeFor[*float64](), reflect.TypeFor[*float32](),
			reflect.TypeFor[*int](), reflect.TypeFor[*int8](), reflect.TypeFor[*int16](), reflect.TypeFor[*int32](), reflect.TypeFor[*int64]():
			// All JSON numbers are float64; JSON unmarshal() will convert the float64 to the right struct field number type
			setStructField(s, fis[i], aids.Must(strconv.ParseFloat(inputJsonFieldSlice[0], 64)))

		case reflect.TypeFor[*string]():
			setStructField(s, fis[i], inputJsonFieldSlice[0])

		case reflect.TypeFor[*[]string]():
			setStructField(s, fis[i], inputJsonFieldSlice)

		case reflect.TypeFor[[]string]():
			setStructField(s, fis[i], inputJsonFieldSlice)

		case reflect.TypeFor[*time.Time]():
			aids.Assert(fis[i].format.isSet, fmt.Sprintf("Struct time.Time field %q missing format tag", fis[i].fieldName))
			if format := fis[i].format.value; format == "RFC1123" {
				setStructField(s, fis[i], aids.Must(time.Parse(http.TimeFormat, inputJsonFieldSlice[0])))
			} else {
				setStructField(s, fis[i], aids.Must(time.Parse(format, inputJsonFieldSlice[0])))
			}

		case reflect.TypeFor[*ETag]():
			setStructField(s, fis[i], ETag(inputJsonFieldSlice[0]))

		default:
			panic(fmt.Sprintf("Field type '%v' not supported", fis[i].fieldType))
		}
	}

	// If struct has an 'Unknown' field of type UnknownFields, set it to the unknown fields
	reflect.ValueOf(s).Elem().FieldByName("Unknown").Set(reflect.ValueOf(unknownJsonFieldNames))
	return verifyStructFields(s) // Validate the struct's fields
}

////////////////////////////////////////////////////////////////////////////////////

// verifyStructFields verifies that the fields of struct s (passed-by-pointer) conform to
// the constraints specified in the struct tags. The struct fields must be T or *T where T is:
// bool, float32, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, string, []string, or another struct.
// The supported struct tags are:
// keys with # values: minval, maxval, minlen, maxlen, minitems, maxitems
// keys with string values: enums (comma-separated), regx, format
// If any field is invalid, an error is returned.
func verifyStructFields(s any) error {
	aids.Assert(s != nil, "should never get here")
	structValue := reflect.ValueOf(s)
	if structValue.Kind() == reflect.Pointer {
		if structValue.IsNil() {
			return nil
		} else {
			structValue = structValue.Elem() // Dereference pointer to get struct value
		}
	}

	structType := structValue.Type()
	if structType.Kind() != reflect.Struct {
		return fmt.Errorf("VerifyStructFields: s must be a struct, got %s", structType.Kind())
	}

	isNumPtrNil := func(v any) bool {
		switch pv := v.(type) {
		case *float32:
			return pv == nil
		case *float64:
			return pv == nil
		case *int:
			return pv == nil
		case *int8:
			return pv == nil
		case *int16:
			return pv == nil
		case *int32:
			return pv == nil
		case *int64:
			return pv == nil
		case *uint:
			return pv == nil
		case *uint8:
			return pv == nil
		case *uint16:
			return pv == nil
		case *uint32:
			return pv == nil
		case *uint64:
			return pv == nil
		default:
			panic(fmt.Sprintf("isPtrNil: type %T not supported", v))
		}
	}

	fieldInfos := getFieldInfos(structType)
	for _, fi := range fieldInfos {
		fieldValue := structValue.FieldByName(fi.fieldName) // Find structure's field value (should be a *T)
		switch v := fieldValue.Interface().(type) {
		case *bool, bool:
			break // No validation for bool

		case *float32, *float64:
			if !isNumPtrNil(v) {
				if err := fi.verifyFloat(fi.fieldName, reflect.ValueOf(v).Elem().Float()); aids.IsError(err) {
					return err
				}
			}
		case float32, float64:
			if err := fi.verifyFloat(fi.fieldName, reflect.ValueOf(v).Float()); aids.IsError(err) {
				return err
			}

		case *int, *int8, *int16, *int32, *int64:
			if !isNumPtrNil(v) {
				if err := fi.verifyInt(fi.fieldName, reflect.ValueOf(v).Elem().Int()); aids.IsError(err) {
					return err
				}
			}

		case int, int8, int16, int32, int64:
			if err := fi.verifyInt(fi.fieldName, reflect.ValueOf(v).Int()); aids.IsError(err) {
				return err
			}

		case *uint, *uint8, *uint16, *uint32, *uint64:
			if !isNumPtrNil(v) {
				if err := fi.verifyUint(fi.fieldName, reflect.ValueOf(v).Elem().Uint()); aids.IsError(err) {
					return err
				}
			}

		case uint, uint8, uint16, uint32, uint64:
			if err := fi.verifyUint(fi.fieldName, reflect.ValueOf(v).Uint()); aids.IsError(err) {
				return err
			}

		case *string:
			if v != nil {
				if err := fi.verifyString(fi.fieldName, *v); aids.IsError(err) {
					return err
				}
			}

		case string:
			if err := fi.verifyString(fi.fieldName, v); aids.IsError(err) {
				return err
			}

		case *[]string:
			if v != nil {
				if err := fi.verifyStrings(fi.fieldName, *v); aids.IsError(err) {
					return err
				}
			}

		case []string:
			if err := fi.verifyStrings(fi.fieldName, v); aids.IsError(err) {
				return err
			}

		default:
			switch {
			case slices.Contains([]reflect.Type{reflect.TypeFor[*ETag](), reflect.TypeFor[ETag](), reflect.TypeFor[*time.Time](), reflect.TypeFor[time.Time]()}, fieldValue.Type()):
				break // Etag & time.Time always pass validation

			case fieldValue.Type().Kind() == reflect.Pointer && fieldValue.Type().Elem().Kind() == reflect.Struct:
				// Recursively validate struct fields
				if err := verifyStructFields(fieldValue.Interface()); aids.IsError(err) {
					return fmt.Errorf("field %q: %v", fi.jsonName, err)
				}

			case fieldValue.Type().Kind() == reflect.Struct:
				// Recursively validate struct fields
				if err := verifyStructFields(fieldValue.Interface()); aids.IsError(err) {
					return fmt.Errorf("field %q: %v", fi.jsonName, err)
				}

			default:
				panic(fmt.Sprintf("Field type '%v' not supported", fi.fieldType))
			}
		}
	}
	return nil
}

type optional[T any] struct {
	isSet bool
	value T // only use if isSet == true
}

type fieldInfo struct {
	jsonName           string                   // JSON field name (for all fields)
	fieldName          string                   // Struct field name (for validation)
	fieldType          reflect.Type             // Struct field type
	minval, maxval     optional[float64]        // For all ints, uints, & floats
	minlen, maxlen     optional[int64]          // For string
	minitems, maxitems optional[int64]          // For []string
	enumValues         optional[[]string]       // For string, []string
	regx               optional[*regexp.Regexp] // For string, []string
	format             optional[string]         // For time.Time
}

func (fi *fieldInfo) verifyFloat(name string, mapValue float64) error {
	if fi.minval.isSet && (mapValue < fi.minval.value) {
		return fmt.Errorf("field '%s' violation: value=%f < minval=%f", name, mapValue, fi.minval.value)
	}
	if fi.maxval.isSet && (mapValue > fi.maxval.value) {
		return fmt.Errorf("field '%s' violation: value=%f > maxval=%f", name, mapValue, fi.maxval.value)
	}
	return nil
}

func (fi *fieldInfo) verifyInt(name string, val int64) error {
	if fi.minval.isSet && (val < int64(fi.minval.value)) {
		return fmt.Errorf("field '%s' violation: value=%d < minval=%d", name, val, int64(fi.minval.value))
	}
	if fi.maxval.isSet && (val > int64(fi.maxval.value)) {
		return fmt.Errorf("field '%s' violation: value=%d > maxval=%d", name, val, int64(fi.maxval.value))
	}
	return nil
}

func (fi *fieldInfo) verifyUint(name string, val uint64) error {
	if fi.minval.isSet && (val < uint64(fi.minval.value)) {
		return fmt.Errorf("field '%s' violation: value=%d < minval=%d", name, val, uint64(fi.minval.value))
	}
	if fi.maxval.isSet && (val > uint64(fi.maxval.value)) {
		return fmt.Errorf("field '%s' violation: value=%d > maxval=%d", name, val, uint64(fi.maxval.value))
	}
	return nil
}

func (fi *fieldInfo) verifyLength(name string, length int) error {
	if fi.minlen.isSet && (length < int(fi.minlen.value)) {
		return fmt.Errorf("field '%s' violation: value=%d < minlen=%d", name, length, int(fi.minlen.value))
	}
	if fi.maxlen.isSet && (length > int(fi.maxlen.value)) {
		return fmt.Errorf("field '%s' violation: value=%d < maxlen=%d", name, length, int(fi.maxlen.value))
	}
	return nil
}

func (fi *fieldInfo) verifyString(name string, s string) error {
	if err := fi.verifyLength(name, len(s)); aids.IsError(err) {
		return err
	}
	if fi.enumValues.isSet && !slices.Contains(fi.enumValues.value, s) {
		return fmt.Errorf("field '%s' violation: value=%s != enums=%s", name, s, strings.Join(fi.enumValues.value, ","))
	}
	if fi.regx.isSet && !fi.regx.value.MatchString(s) {
		return fmt.Errorf("field '%s' violation: value=%s != regex=%s", name, s, fi.regx.value.String())
	}
	return nil
}

func (fi *fieldInfo) verifyStrings(name string, s []string) error {
	if fi.minitems.isSet && (len(s) < int(fi.minitems.value)) {
		return fmt.Errorf("field '%s' violation: value=%d != minitems=%d", name, len(s), int(fi.minitems.value))
	}
	if fi.maxitems.isSet && (len(s) > int(fi.maxitems.value)) {
		return fmt.Errorf("field '%s' violation: value=%d != maxitems=%d", name, len(s), int(fi.maxitems.value))
	}
	for _, eachString := range s {
		if err := fi.verifyString(fmt.Sprintf("%s[%s]", name, eachString), eachString); aids.IsError(err) {
			return err
		}
	}
	return nil
}

func tagTo[T int64 | float64 | string | []string | *regexp.Regexp](tag reflect.StructTag, key string, convert func(val string) T) optional[T] {
	if val, ok := tag.Lookup(key); !ok {
		return optional[T]{}
	} else {
		return optional[T]{isSet: true, value: convert(val)}
	}
}

func getFieldInfos(structType reflect.Type) []fieldInfo {
	// dereference returns the underlying type if t is a pointer type, otherwise returns t itself
	dereference := func(t reflect.Type) reflect.Type {
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		return t
	}
	structType = dereference(structType)
	aids.Assert(structType.Kind() == reflect.Struct, "structType must be a struct")
	if fieldInfos, ok := typeToFieldInfos.Load(structType); ok { // Not in cache
		return fieldInfos
	}

	parseInt64 := func(val string) int64 { return aids.Must(strconv.ParseInt(val, 10, 64)) }
	parseFloat64 := func(val string) float64 { return aids.Must(strconv.ParseFloat(val, 64)) }

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
			jsonName:   propName,
			fieldName:  structField.Name,
			fieldType:  structField.Type,
			minval:     tagTo(tag, "minval", parseFloat64),
			maxval:     tagTo(tag, "maxval", parseFloat64),
			minlen:     tagTo(tag, "minlen", parseInt64),
			maxlen:     tagTo(tag, "maxlen", parseInt64),
			minitems:   tagTo(tag, "minitems", parseInt64),
			maxitems:   tagTo(tag, "maxitems", parseInt64),
			enumValues: tagTo(tag, "enums", func(val string) []string { return strings.Split(val, ",") }),
			regx:       tagTo(tag, "regx", func(val string) *regexp.Regexp { return regexp.MustCompile(val) }),
			format:     tagTo(tag, "format", func(val string) string { return val }),
		}
		fieldInfos = append(fieldInfos, fi)
	}
	typeToFieldInfos.Store(structType, fieldInfos) // cache it for future use
	return fieldInfos
}

var typeToFieldInfos = syncmap.Map[reflect.Type, []fieldInfo]{}
