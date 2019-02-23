package streams

import (
	"errors"
	"reflect"
)

func checkChan(channel interface{}) reflect.Type {
	t := reflect.TypeOf(channel)
	if t.Kind() != reflect.Chan || t.ChanDir()&reflect.RecvDir == 0 {
		panic(errors.New("provided type is not <-chan"))
	}
	return t
}

func chanRecv(channel interface{}) (interface{}, bool) {
	t := checkChan(channel)

	if t.Elem().Kind() == reflect.Interface {
		// Already <-chan interface{}
		item, ok := <-channel.(<-chan interface{})
		return item, ok
	}

	val := reflect.ValueOf(channel)
	item, ok := val.Recv()
	return item.Interface(), ok
}
