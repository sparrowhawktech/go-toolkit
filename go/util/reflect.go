package util

import (
	"encoding/json"
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})
var byteArrayType = reflect.TypeOf([]byte{})
var rawMessageType = reflect.TypeOf(json.RawMessage{})

func IsStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t != timeType
}

func IsStructPtr(value reflect.Value) bool {
	if value.Kind() == reflect.Ptr {
		instanceType := value.Type().Elem()
		return instanceType.Kind() == reflect.Struct && instanceType != timeType
	} else {
		return false
	}
}

func IsStructPtrType(t reflect.Type) bool {
	if t.Kind() == reflect.Ptr {
		instanceType := t.Elem()
		return instanceType.Kind() == reflect.Struct && instanceType != timeType
	} else {
		return false
	}
}

func IsStructValue(value reflect.Value) bool {
	return IsStruct(value.Type())
}

func CanIsNil(value reflect.Value) bool {
	k := value.Kind()
	return k == reflect.Ptr || k == reflect.Chan || k == reflect.Map || k == reflect.Slice || k == reflect.Func
}

func IsArray(t reflect.Type) bool {
	kind := t.Kind()
	return (kind == reflect.Slice || kind == reflect.Array) && t != byteArrayType && t != rawMessageType
}

func IsTimeValue(value reflect.Value) bool {
	return IsTime(value.Type())
}

func IsTime(t reflect.Type) bool {
	return t == timeType
}
