package streams

import (
	"errors"
	"reflect"
)

func checkSlice(slice interface{}) reflect.Type {
	t := reflect.TypeOf(slice)
	if t.Kind() != reflect.Slice {
		panic(errors.New("provided type is not a slice"))
	}
	return t
}

func sliceIndex(slice interface{}, index int) interface{} {
	t := checkSlice(slice)

	if t.Elem().Kind() == reflect.Interface {
		// Already []interface{}
		return slice.([]interface{})[index]
	}

	val := reflect.ValueOf(slice)
	return val.Index(index).Interface()
}

func sliceLength(slice interface{}) int {
	checkSlice(slice)

	val := reflect.ValueOf(slice)
	return val.Len()
}
