package streams

import (
	"errors"
	"reflect"
)

func checkSlice(slice AnySlice) reflect.Type {
	t := reflect.TypeOf(slice)
	if t.Kind() != reflect.Slice {
		panic(errors.New("provided type is not a slice"))
	}
	return t
}

func sliceIndex(slice AnySlice, index int) AnyType {
	t := checkSlice(slice)

	if t.Elem().Kind() == reflect.Interface {
		// Already []interface{}
		return slice.([]interface{})[index]
	}

	val := reflect.ValueOf(slice)
	return val.Index(index).Interface()
}

func sliceLen(slice AnySlice) int {
	checkSlice(slice)

	val := reflect.ValueOf(slice)
	return val.Len()
}
