// Package streams provides the Stream type, which is a lazily evaluated chain of functions which operates on some
// source of values. Streams allows you to define a pipeline of operations to perform on a source of iterated values.
// The pieces of the pipeline are lazily evaluated, so for example, items which fail a Filter operation will not be
// passed to a following Map operation (or any operation).
//
// Everything in this package is accessed through a Stream object. Create Stream objects with either NewChanStream or
// NewSliceStream functions.
//
// Channel streams allow infinite value suppliers which a Stream can use to process as much as needed. Read the
// documentation on Stream itself for more information regarding infinite Streams.
//
// Streams are generic in nature, Streams have an implicit type which relates to the types of the items passed to the
// functions given to the Stream. Unfortunately, Go does not have any mechanism for defining generic types or functions.
// Because of this, the "catch-all" type of interface{} is used as the input parameter for most methods on the Stream
// type. It is vital that the actual type of the functions passed to the methods of Stream are correct, though, and the
// compiler will not assist with this. It is important that you read the documentation for each method to know which
// type of function is required.
//
// Streams may be backed by channels which may be sourced through a running goroutine. In this case, you may want to
// cancel any running goroutine involved with the Stream when the Stream is done processing. Streams support 'cancel'
// channels which will be send a single 'true' value when the Stream operation completes. Pass these to the Stream
// object either through the additional arguments to NewChanStream or with the WithCancel method.
//
// Streams are typically used in a fluent way. That is, the output of one Stream operation isn't stored in a variable,
// instead further operations in the pipeline are defined directly on the returned object until the final operation is
// called. Note that because Streams are lazily evaluated, calling a non-terminating method on Stream does not actually
// process any data. If a Stream is defined without calling a terminating method, no data will be processed.
//
// Take the following example on how Streams can be used with a slice as the data source:
//
//     func countLetters(data []string) int {
//         return streams.NewSliceStream(s).
//             Filter(func(text string) bool {
//                 return !strings.ContainsRune(text, ' ')
//             }).
//             Map(func(word string) string {
//                 return strings.ToUpper(word)
//             }).
//             SliceFlatMap(func(word string) []rune {
//                 return []rune(word)
//             }).
//             Filter(func(char rune) bool {
//                 return unicode.IsLetter(char)
//             }).
//             Count()
//     }
//
// In this example, the Stream pipeline counts the number of letters in every string in the given slice that doesn't
// contain a space character. It's probably not a very realistic example, but hopefully it will make it clear the syntax
// on how a Stream should be used.
package streams

import (
	"errors"
	"math"
	"reflect"
	"sort"
)

// MapFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T, R> func(T) R
//
// Where there is some type T as input to the function, and some type R as output. If this type signature is not
// maintained where this function is used, a panic will occur.
type MapFunction interface{}

// FilterFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T> func(T) bool
//
// Where there is some type T as input to the function, and a single bool as output. If this type signature is not
// maintained where this function is used, a panic will occur.
type FilterFunction interface{}

// VoidFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T> func(T)
//
// Where there is some type T as input to the function, and no output. If this type signature is not maintained where
// this function is used, a panic will occur.
type VoidFunction interface{}

// ChanMapFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T, R> func(T) <-chan R
//
// Where there is some type T as input to the function, and a receiver channel of some type R as output. If this type
// signature is not maintained where this function is used, a panic will occur.
type ChanMapFunction interface{}

// SliceMapFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T, R> func(T) []R
//
// Where there is some type T as input to the function, and a slice of some type R as output. If this type
// signature is not maintained where this function is used, a panic will occur.
type SliceMapFunction interface{}

// CompareFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T> func(left, right T) bool
//
// Where there is some type T which is the same type for 2 input parameters to the function, and a single bool as
// output. If this type signature is not maintained where this function is used, a panic will occur.
//
// The function should return true if the left parameter should be considered smaller, or should come before, the right
// parameter.
type CompareFunction interface{}

// MapToIntFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T, I : int> func(T) I
//
// Where there is some type T as input to the function, and an int type as output. The necessary int type is defined by
// the function which takes this as input. If this type signature is not maintained where this function is used, a
// panic will occur.
type MapToIntFunction interface{}

// MapToFloatFunction is an empty stand-in type for a generic function with a type signature as
//
//     <T, F : float> func(T) F
//
// Where there is some type T as input to the function, and a float type as output. The necessary float type is defined
// by the function which takes this as input. If this type signature is not maintained where this function is used, a
// panic will occur.
type MapToFloatFunction interface{}

// Stream represents a lazily evaluated chain of functions which operates on some source of values. Items are computed
// as they are asked for and as they go through the Stream pipeline, so if an items doesn't need to be processed by
// later parts of the Stream, it is skipped.
//
// There are two ways of creating a Stream: with a channel or with a slice.
//
// To create a channel-based Stream, use the NewChanStream function. Channel based Streams have the benefit of
// allowing the source to be an infinite value generator. In cases where infinite generators are used, it is essential
// that the total amount of items processed is limited with Take or First. If First is used, Sort may never be used
// on an infinite Stream. If Last is used, Sort may only be used after Sort in the Stream pipeline.
//
// To create a slice-based Stream, use the NewSliceStream function. Slice based Streams are limited to the size of the
// slice and cannot be infinite.
//
// Due to Go's lack of any generic type functionality, type safety is entirely up to the programmer. To allow functions
// to be used with precise types, input types for these methods must be the most vague possible type, interface{}. This
// means the compiler will not catch type issues if any type is passed to a Stream method, so the programmer must pay
// much closer attention. Any given Stream has an implicit "type". This type is the type of items that will be passed to
// any input function that's passed to this Stream. The input types for functions passed to a Stream must always match
// this implicit type of a Stream. Mapping operations return Streams with new implicit types, so as the Stream pipeline
// continues, the implicit type changes.
//
// For example, with a Stream created:
//
//     slice := []string{"foo", "bar"}
//     s := streams.NewSliceStream(slice)
//
// The Stream 's' would have an implicit type of 'string'. If you did a mapping operation:
//
//     s1 := s.Map(func(word string) int {
//         return len(word)
//     })
//
// Then the resulting Stream 's1' would have an implicit type of 'int'. Note that Stream sources can only be evaluated
// once, so it usually doesn't make sense to assign each operation to a different value, so the above could bbe instead
// written as:
//
//     1 slice := []string{"foo", "bar"}
//     2 s := streams.NewSliceStream(slice).
//     3     Map(func(word string) int {
//     4         return len(word)
//     5     })
//
// In this case, the Stream returned from 'streams.NewSliceStream()' on line 2 has an implicit type of 'string', and the
// Stream returned from 'Map()' on line 3 has an implicit type of 'int', so the final Stream assigned to 's' also has
// an implicit type of 'int'.
type Stream struct {
	next   func() (interface{}, bool)
	cancel *[]chan<- bool
}

// NewChanStream creates a new Stream object that uses the provided channel as the source. The first argument to this
// function must be a <-chan R where R is some type. The implicit type of the returned Stream will be R.
//
// The provided channel may be an infinite value generator. In this case, you must make sure to use limiting functions
// like Take or First to prevent the Stream from processing forever and crashing.
//
// Any arguments provided after the stream are channels which should be used to stop any running goroutine which needs
// to be stopped when processing of the Stream completes. A single 'true' value will be sent to each channel given. The
// send operation will not wait or block, so either define each channel as a buffered channel, or make sure you're
// always listening to it.
func NewChanStream(channel interface{}, cancel ...chan<- bool) *Stream {
	return &Stream{func() (interface{}, bool) {
		item, ok := chanRecv(channel)
		if ok {
			return item, true
		}

		return nil, false
	}, &cancel}
}

// NewSliceStream creates a new Stream object that uses the provided slice as the source. Teh first argument to this
// function must be a []R where R is some type. The implicit type of the returned Stream will be R.
func NewSliceStream(slice interface{}) *Stream {
	index := 0
	return &Stream{func() (interface{}, bool) {
		if index < sliceLength(slice) {
			item := sliceIndex(slice, index)
			index++
			return item, true
		}
		return nil, false
	}, &[]chan<- bool{}}
}

func callFunc(f interface{}, args ...reflect.Value) []reflect.Value {
	t := reflect.TypeOf(f)
	if t.Kind() != reflect.Func {
		panic(errors.New("provided type is not func"))
	}

	return reflect.ValueOf(f).Call(args)
}

func (s *Stream) finish() {
	for _, c := range *s.cancel {
		if c != nil {
			select {
			case c <- true:
			default:
			}
		}
	}
}

// Map takes in a mapping function and returns a Stream whose elements are the elements of this Stream passed
// through the given mapping function.
//
// The given mapping function must have the type signature of:
//
//     <T, R> func(T) R
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur. The return type of this mapping function determines the new
// type for the elements in the returned Stream.
func (s *Stream) Map(mapperFunc MapFunction) *Stream {
	return &Stream{func() (interface{}, bool) {
		n, more := s.next()
		if !more {
			return nil, false
		}
		return callFunc(mapperFunc, reflect.ValueOf(n))[0].Interface(), true
	}, s.cancel}
}

// Filter takes in a filtering function and returns a Stream whose elements are the elements of this Stream that
// satisfy the given filtering function. When the function returns true, the element passes through. When the
// function returns false, the element is not allowed through.
//
// The given filtering function must have a type signature of:
//
//     <T> func(T) bool
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) Filter(filterFunc FilterFunction) *Stream {
	return &Stream{func() (interface{}, bool) {
		n, more := s.next()
		for more {
			if callFunc(filterFunc, reflect.ValueOf(n))[0].Interface().(bool) {
				return n, true
			}
			n, more = s.next()
		}
		return nil, false
	}, s.cancel}
}

// ChanFlatMap takes in a mapping function and returns a Stream whose elements are defined by the channel returned
// by the given mapping function. For example, if one element is passed to the mapping function, and the channel
// returned from the mapping function provides 2 elements, these 2 elements will be the elements of the returned
// Stream.
//
// The given mapping function must have a type signature of:
//
//     <T, R> func(T) <-chan R
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur. The type of the returned channel from this mapping function
// determines the new type for the elements in the returned Stream.
//
// For example, if the provided mapping function is
//
//     func(s string) <-chan rune
//
// then the returned Stream will process elements of type rune.
func (s *Stream) ChanFlatMap(mapperFunc ChanMapFunction) *Stream {
	var currentChan interface{}

	nextChan := func() {
		if currentChan == nil {
			n, more := s.next()
			if !more {
				return
			}
			currentChan = callFunc(mapperFunc, reflect.ValueOf(n))[0].Interface()
		}
	}

	nextItem := func() (res interface{}, retry, more bool) {
		nextChan()

		if currentChan == nil {
			return nil, false, false
		}

		next, ok := chanRecv(currentChan)
		if !ok {
			currentChan = nil
			return nil, true, true
		}

		return next, false, true
	}

	return &Stream{func() (interface{}, bool) {
		res, retry, more := nextItem()
		if !more {
			return nil, false
		}
		for retry {
			res, retry, more = nextItem()
			if !more {
				return nil, false
			}
		}
		return res, true
	}, s.cancel}
}

// SliceFlatMap takes in a mapping function and returns a Stream whose elements are defined by the slice returned
// by teh given mapping function. For example, if one element is passed to the mapping function, and the slice
// returned from the mapping function contains 2 elements, these 2 elements will be the elements of the returned
// Stream.
//
// The given mapping function must have a type signature of:
//
//     <T, R> func(T) []R
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur. The type of the returned slice from this mapping function
// determines the new type for the elements of the returned Stream.
//
// For example, if the provided mapping function is
//
//     func(s string) []rune
//
// then the returned Stream will process elements of type rune.
func (s *Stream) SliceFlatMap(mapperFunc SliceMapFunction) *Stream {
	return s.ChanFlatMap(func(item interface{}) <-chan interface{} {
		if item == nil {
			return nil
		}

		slice := callFunc(mapperFunc, reflect.ValueOf(item))[0].Interface()
		resChan := make(chan interface{})

		go func() {
			length := sliceLength(slice)
			for i := 0; i < length; i++ {
				el := sliceIndex(slice, i)
				resChan <- el
			}
			close(resChan)
		}()

		return resChan
	})
}

// Take returns a Stream that only passes along the first n elements it sees. After either the source Stream stops
// providing more items, or the source Stream has provided n items, this Stream will stop providing more items.
//
// This can be useful if processing data from an infinite channel, the Stream process will never complete unless you
// either call this function or call First to prevent the final Stream from continually processing items.
func (s *Stream) Take(n int) *Stream {
	count := 0
	return &Stream{func() (interface{}, bool) {
		if count >= n {
			return nil, false
		}

		item, more := s.next()
		if !more {
			return nil, false
		}
		count++
		return item, true
	}, s.cancel}
}

// Skip returns a Stream that skips the first n elements it sees before passing along any elements. If the Stream never
// sees n elements, this Stream will never pass along any items.
func (s *Stream) Skip(n int) *Stream {
	count := 0
	return &Stream{func() (interface{}, bool) {
		if count >= n {
			return s.next()
		}
		for count < n {
			// Ignore these
			_, more := s.next()
			if !more {
				return nil, false
			}
			count++
		}
		return s.next()
	}, s.cancel}
}

// Distinct returns a Stream that only passes along items that haven't been seen before. After seeing an item pass
// through, that item will no longer pass through if it is provided again by the source Stream.
//
// The equality check for items uses map[interface{}]bool keys.
func (s *Stream) Distinct() *Stream {
	m := make(map[interface{}]bool)

	return &Stream{func() (interface{}, bool) {
		for {
			item, more := s.next()
			if !more {
				return nil, false
			}
			if m[item] {
				continue
			}
			m[item] = true
			return item, true
		}
	}, s.cancel}
}

type sortable struct {
	data     []interface{}
	compFunc interface{}
}

func (s *sortable) Len() int {
	return len(s.data)
}

func (s *sortable) Swap(i, j int) {
	s.data[i], s.data[j] = s.data[j], s.data[i]
}

func (s *sortable) Less(i, j int) bool {
	left := reflect.ValueOf(s.data[i])
	right := reflect.ValueOf(s.data[j])
	return callFunc(s.compFunc, left, right)[0].Bool()
}

// Sort returns a Stream where every item is in sorted order defined by the given comparison function.
//
// The given comparison function must have a type signature of:
//
//     <T> func(left, right T) bool
//
// And the input type T must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
//
// The given function should return true if the left parameter should be considered smaller, or should come before, the
// right parameter.
//
// Due to the nature of sorting, this is a pausing operation. That is to say, this operation waits until every item
// has been seen before continuing. Due to this, if using an infinite source, you must limit the total amount of
// items with Take() or this function will never complete.
func (s *Stream) Sort(lessFunc CompareFunction) *Stream {
	var (
		sorted []interface{} = nil
		index                = 0
	)

	doSort := func() {
		var data []interface{}
		s.ToSlice(&data)

		sortableData := &sortable{data, lessFunc}
		sort.Sort(sortableData)

		sorted = sortableData.data
	}

	return &Stream{func() (interface{}, bool) {
		if sorted == nil {
			doSort()
		}

		if index >= len(sorted) {
			return nil, false
		}

		item := sorted[index]
		index++
		return item, true
	}, s.cancel}
}

// OnEach returns a Stream where every element in the Stream is passed through the given function first before
// continuing. The function returns nothing and does not modify the element. This is similar to ForEach, but is an
// intermediate operation.
//
// The given function must have a type signature of:
//
//     <T> func(T)
//
// And the input type T must be compatible with ever element in the Stream that makes it to this function. If this type
// signature isn't correct, a panic will occur.
func (s *Stream) OnEach(voidFunc VoidFunction) *Stream {
	return &Stream{func() (interface{}, bool) {
		n, more := s.next()
		if !more {
			return nil, false
		}
		callFunc(voidFunc, reflect.ValueOf(n))
		return n, true
	}, s.cancel}
}

// WithCancel takes in a sendable channel which takes a bool to signify that the Stream process has completed. Use
// this any time you have created a goroutine which should be stopped when the Stream has completed processing. The
// final Stream will send true to every cancelling channel given when a final operation occurs.
func (s *Stream) WithCancel(c chan<- bool) *Stream {
	cancels := append(*s.cancel, c)
	return &Stream{s.next, &cancels}
}

// First returns the first element in this Stream that satisfies the given filtering function. When the function
// returns true, the element will be returned. When the function returns false, the element is skipped.
//
// The given filtering function must have a type signature of:
//
//     <T> func(T) bool
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) First(filterFunc FilterFunction) (interface{}, bool) {
	defer s.finish()

	for {
		n, more := s.next()
		if !more {
			return nil, false
		}
		if callFunc(filterFunc, reflect.ValueOf(n))[0].Bool() {
			return n, true
		}
	}
}

// ToSlice fills the given slice with the elements in the Stream. The slice type must be compatible with every item
// in the Stream. The input of this function must be a pointer to the slice, rather than the slice itself, so the
// slice may be resized as necessary.
func (s *Stream) ToSlice(t interface{}) {
	defer s.finish()

	sliceValue := reflect.ValueOf(t).Elem()

	for {
		n, more := s.next()
		if !more {
			return
		}
		sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(n)))
	}
}

// Count returns the number of elements in this Stream. Cannot be called on an infinite Stream.
func (s *Stream) Count() int {
	defer s.finish()

	var i = 0
	for {
		_, more := s.next()
		if !more {
			return i
		}
		i++
	}
}

// Any returns true if there are any items in this Stream which satisfies the given filtering function. When the
// function returns true, true will be returned. When the function returns false, the item will be skipped and
// others will be tested. If no items pass, false will be returned.
//
// The given filtering function must have a type signature of:
//
//     <T> func(T) bool
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) Any(filterFunc FilterFunction) bool {
	defer s.finish()

	for {
		n, more := s.next()
		if !more {
			return false
		}
		if callFunc(filterFunc, reflect.ValueOf(n))[0].Bool() {
			return true
		}
	}
}

// None returns true if there are no items in this Stream which satisfies the given filtering function. When the
// function returns true, false will be returned. When the function returns true, the item will be skipped and
// others will be tested. If no items pass, true will be returned.
//
// The given filtering function must have a type signature of:
//
//     <T> func(T) bool
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) None(filterFunc FilterFunction) bool {
	return !s.Any(filterFunc)
}

// All returns true if all items in this Stream which satisfies the given filtering function. When the function
// returns false, false will be returned. When the function returns true, the item will be skipped and
// others will be tested. If all items pass, true will be returned.
//
// The given filtering function must have a type signature of:
//
//     <T> func(T) bool
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) All(filterFunc FilterFunction) bool {
	defer s.finish()

	for {
		n, more := s.next()
		if !more {
			return true
		}
		if !callFunc(filterFunc, reflect.ValueOf(n))[0].Bool() {
			return false
		}
	}
}

// ForEach runs the given function with each element in the Stream that makes it to this function.
//
// The given function must have a type signature of:
//
//     <T> func(T)
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this type
// signature isn't correct, a panic will occur.
func (s *Stream) ForEach(voidFunc VoidFunction) {
	defer s.finish()

	for {
		n, more := s.next()
		if !more {
			return
		}
		callFunc(voidFunc, reflect.ValueOf(n))
	}
}

// ToChan sends the elements of this Stream to the given channel. The channel must be compatible with the type of every
// element in this Stream. If the given channel is not compatible with an element in this Stream then a panic will
// occur.
//
// When no more items are to be sent to the channel, the given channel will be closed.
func (s *Stream) ToChan(channel interface{}) {
	defer s.finish()

	t := reflect.TypeOf(channel)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Chan || t.ChanDir()&reflect.SendDir == 0 {
		panic(errors.New("provided type is not chan<- T"))
	}

	val := reflect.ValueOf(channel)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	for {
		n, more := s.next()
		if !more {
			val.Close()
			return
		}
		val.Send(reflect.ValueOf(n))
	}
}

// SumInt returns the sum of the items in this Stream converted to int64 using the given mapping function.
//
// The given mapping function must have a type signature of:
//
//     <T> func(T) int64
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) SumInt(mapperFunc MapToIntFunction) int64 {
	defer s.finish()

	var res int64 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, reflect.ValueOf(v))[0].Int()
	}
	return res
}

// SumFloat returns the sum of the items in this Stream converted to float64 using the given mapping function.
//
// The given mapping function must have a type signature of:
//
//     <T> func(T) float64
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) SumFloat(mapperFunc MapToFloatFunction) float64 {
	defer s.finish()

	var res float64 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, reflect.ValueOf(v))[0].Float()
	}
	return res
}

// AvgInt returns the average of the items in this Stream converted to int64 using the given mapping function.
//
// The given mapping function must have a type signature of:
//
//     <T> func(T) int64
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) AvgInt(mapperFunc MapToIntFunction) int64 {
	defer s.finish()

	var (
		sum   int64 = 0
		count       = 0
	)

	for {
		item, more := s.next()
		if !more {
			break
		}

		sum += callFunc(mapperFunc, reflect.ValueOf(item))[0].Int()
		count++
	}

	return int64(math.Round(float64(sum) / float64(count)))
}

// AvgFloat returns the average of the items in this Stream converted to float64 using the given mapping function.
//
// The given mapping function must have a type signature of:
//
//     <T> func(T) float64
//
// And the input type must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
func (s *Stream) AvgFloat(mapperFunc MapToFloatFunction) float64 {
	defer s.finish()

	var (
		sum   float64 = 0
		count         = 0
	)

	for {
		item, more := s.next()
		if !more {
			break
		}

		sum += callFunc(mapperFunc, reflect.ValueOf(item))[0].Float()
		count++
	}

	return sum / float64(count)
}

// Min finds the smallest value in this Stream based on the given comparison function.
//
// The given comparison function must have a type signature of:
//
//    <T> func(left, right T) bool
//
// And the input type T must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
//
// The given function should return true if the left parameter should be considered smaller, or should come before, the
// right parameter.
func (s *Stream) Min(output interface{}, lessFunc CompareFunction) {
	defer s.finish()

	var smallest *reflect.Value

	for {
		item, more := s.next()
		if !more {
			break
		}

		if smallest == nil {
			val := reflect.ValueOf(item)
			smallest = &val
		} else {
			val := reflect.ValueOf(item)
			if !callFunc(lessFunc, *smallest, val)[0].Bool() {
				smallest = &val
			}
		}
	}

	if smallest == nil {
		return
	}

	t := reflect.TypeOf(output)
	if t.Kind() != reflect.Ptr {
		panic(errors.New("provided output type is not a pointer"))
	}

	reflect.ValueOf(output).Elem().Set(*smallest)
}

// Max finds the largest value in this Stream based on the given comparison function.
//
// The given comparison function must have a type signature of:
//
//    <T> func(left, right T) bool
//
// And the input type T must be compatible with every element in the Stream that makes it to this function. If this
// type signature isn't correct, a panic will occur.
//
// The given function should return true if the left parameter should be considered smaller, or should come before, the
// right parameter.
func (s *Stream) Max(output interface{}, lessFunc CompareFunction) {
	s.Min(output, func(left, right interface{}) bool {
		return callFunc(lessFunc, reflect.ValueOf(right), reflect.ValueOf(left))[0].Bool()
	})
}
