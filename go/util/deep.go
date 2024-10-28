package util

import (
	"reflect"
)

func CopyStructGraph[T any](t T) T {
	v := reflect.ValueOf(t)
	if CanIsNil(v) && v.IsNil() {
		return t
	}
	isPointer := v.Kind() == reflect.Pointer
	if isPointer {
		v = v.Elem()
	}
	v2 := CopyStructValue(v)
	if isPointer {
		v2 = v2.Addr()
	}
	return v2.Interface().(T)
}

// CopyStructValue For Pointer to Pointer (**) members the value referenced by the second level
// pointer is NOT cloned It is assumed if you declared a pointer to pointer that's
// exactly what you want. Notice this applies to pointers to maps and slices as
// well, since they are pointer themselves. Pointers to primitive values are
// neither cloned into a new pointer. If you are running on snapshots and messing
// around with POINTERS TO MEMBERS, you are doing soethign really nasty and we
// assume you are aware, then you are on your own. Note that for the general case
// of pointer members and primitive values, if you change the field value you are
// assigning a new pointer to it and this as safe as it gets. You are not really
// sharing anything.
func CopyStructValue(value reflect.Value) reflect.Value {
	valueType := value.Type()
	if !IsStruct(valueType) && !IsStructPtrType(valueType) {
		panic("Only structs and pointers to struct supported. May be you are looking for CopySliceValue or CopyMapValue?")
	}
	newValue := reflect.New(valueType).Elem()
	for i := 0; i < value.NumField(); i++ {
		f := value.Field(i)
		nf := newValue.Field(i)
		nv := copyValue(f)
		nf.Set(nv)
	}
	return newValue
}

func copyValue(f reflect.Value) reflect.Value {
	if CanIsNil(f) && f.IsNil() {
		return reflect.ValueOf(f.Interface())
	}
	if f.Kind() == reflect.Map {
		nv := CopyMapValue(f)
		return nv
	} else if f.Kind() == reflect.Slice {
		nv := CopySliceValue(f)
		return nv
	} else if IsStructPtr(f) && !f.IsNil() {
		nv := CopyStructValue(f.Elem())
		return nv.Addr()
	} else if IsStructValue(f) {
		nv := CopyStructValue(f)
		return nv
	} else {
		return f
	}
}

func CopySliceValue(f reflect.Value) reflect.Value {
	elemType := f.Type().Elem()
	nv := reflect.MakeSlice(reflect.SliceOf(elemType), f.Len(), f.Len())
	isPointer := elemType.Kind() == reflect.Pointer
	if isGraph(elemType) {
		for i := 0; i < f.Len(); i++ {
			v := f.Index(i)
			if v.IsNil() {
				nv.Index(i).Set(v)
			} else {
				if isPointer {
					v = v.Elem()
				}
				v2 := copyValue(v)
				if isPointer {
					v2 = v2.Addr()
				}
				nv.Index(i).Set(v2)
			}
		}
	} else {
		reflect.Copy(nv, f)
	}
	return nv
}

func CopyMap[T any](m any) T {
	return CopyMapValue(reflect.ValueOf(m)).Interface().(T)
}

func CopyMapValue(f reflect.Value) reflect.Value {
	elemType := f.Type().Elem()
	nv := reflect.MakeMap(reflect.MapOf(f.Type().Key(), elemType))
	deep := isGraph(elemType)
	isPointer := elemType.Kind() == reflect.Pointer
	for _, k := range f.MapKeys() {
		v := f.MapIndex(k)
		if deep && !v.IsNil() {
			if isPointer {
				v = v.Elem()
			}
			v2 := copyValue(v)
			if isPointer {
				v2 = v2.Addr()
			}
			nv.SetMapIndex(k, v2)
		} else {
			nv.SetMapIndex(k, v)
		}
	}
	return nv
}

func isGraph(t reflect.Type) bool {
	k := t.Kind()
	return k == reflect.Map || k == reflect.Slice || IsStructPtrType(t)
}
