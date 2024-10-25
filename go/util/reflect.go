package util

import (
	"reflect"
	"time"
)

var timeType = reflect.TypeOf(time.Time{})

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
