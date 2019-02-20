package streams

import (
	"errors"
	"math"
	"reflect"
	"sort"
)

type Stream interface {
	Map(mapperFunc interface{}) Stream          // <T, R> func(T) R
	Filter(filterFunc interface{}) Stream       // <T> func(T) bool
	ChanFlatMap(mapperFunc interface{}) Stream  // <T, R> func(T) <-chan R
	SliceFlatMap(mapperFunc interface{}) Stream // <T, R> func(T) []R
	Take(n int) Stream
	Distinct() Stream
	Sort(lessFunc interface{}) Stream // <T> func(left, right T) bool

	WithCancel(c chan<- bool) Stream

	First(filter interface{}) interface{} // <T> func(T) bool
	ToSlice(t interface{})
	Any(filter interface{}) bool  // <T> func(T) bool
	None(filter interface{}) bool // <T> func(T) bool
	All(filter interface{}) bool  // <T> func(T) bool

	SumInt32(mapperFunc interface{}) int32     // <T> func(T) int32
	SumInt64(mapperFunc interface{}) int64     // <T> func(T) int64
	SumFloat32(mapperFunc interface{}) float32 // <T> func(T) float32
	SumFloat64(mapperFunc interface{}) float64 // <T> func(T) float64
	AvgInt(mapperFunc interface{}) int64       // <T> func(T) int64
	AvgFloat(mapperFunc interface{}) float64   // <T> func(T) float64
}

type stream struct {
	next   func() (interface{}, bool)
	cancel *[]chan<- bool
}

func NewChanStream(channel interface{}, cancel ...chan<- bool) Stream {
	generic, c := generifyChannel(channel)

	cancels := make([]chan<- bool, len(cancel)+1)
	copy(cancels, cancel)
	cancels[len(cancel)] = c

	return &stream{func() (interface{}, bool) {
		item, ok := <-generic
		if ok {
			return item, true
		}

		return nil, false
	}, &cancels}
}

func NewSliceStream(slice interface{}) Stream {
	generic := generifySlice(slice)

	index := 0

	return &stream{func() (interface{}, bool) {
		if index < len(generic) {
			item := generic[index]
			index++
			return item, true
		}
		return nil, false
	}, &[]chan<- bool{}}
}

func generifySlice(slice interface{}) []interface{} {
	t := reflect.TypeOf(slice)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Slice {
		panic(errors.New("provided type is not a slice"))
	}

	if t.Elem().Kind() == reflect.Interface {
		// Already []interface{}
		return slice.([]interface{})
	}

	val := reflect.ValueOf(slice)

	length := val.Len()
	res := make([]interface{}, length)

	for i := 0; i < length; i++ {
		res[i] = val.Index(i).Interface()
	}

	return res
}

func generifyChannel(c interface{}) (<-chan interface{}, chan<- bool) {
	t := reflect.TypeOf(c)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Chan || t.ChanDir()&reflect.RecvDir == 0 {
		panic(errors.New("provided type is not <-chan"))
	}

	if t.Elem().Kind() == reflect.Interface {
		// Already <-chan interface{}
		return c.(<-chan interface{}), nil
	}

	val := reflect.ValueOf(c)

	res := make(chan interface{})
	cancel := make(chan bool, 1)

	go func() {
		cases := []reflect.SelectCase{
			{reflect.SelectRecv, val, reflect.Value{}},
			{reflect.SelectRecv, reflect.ValueOf(cancel), reflect.Value{}},
		}
		for {
			i, item, ok := reflect.Select(cases)
			if !ok || i == len(cases)-1 {
				close(res)
				return
			}

			select {
			case res <- item.Interface():
			case <-cancel:
				close(res)
				return
			}
		}
	}()

	return res, cancel
}

func convertToValues(args []interface{}) []reflect.Value {
	res := make([]reflect.Value, len(args))

	for i, v := range args {
		res[i] = reflect.ValueOf(v)
	}

	return res
}

func convertToInterfaces(v []reflect.Value) []interface{} {
	res := make([]interface{}, len(v))

	for i, t := range v {
		res[i] = t.Interface()
	}

	return res
}

func callFunc(f interface{}, args ...interface{}) []interface{} {
	t := reflect.TypeOf(f)
	if t.Kind() != reflect.Func {
		panic(errors.New("provided type is not func"))
	}

	res := reflect.ValueOf(f).Call(convertToValues(args))
	return convertToInterfaces(res)
}

func (s *stream) finish() {
	for _, c := range *s.cancel {
		if c != nil {
			select {
			case c <- true:
			default:
			}
		}
	}
}

func (s *stream) Map(mapperFunc interface{}) Stream { // <T, R> func(T) R
	return &stream{func() (interface{}, bool) {
		n, more := s.next()
		if !more {
			return nil, false
		}
		return callFunc(mapperFunc, n)[0], true
	}, s.cancel}
}

func (s *stream) Filter(filterFunc interface{}) Stream { // <T> func(T) bool
	return &stream{func() (interface{}, bool) {
		n, more := s.next()
		for more {
			if callFunc(filterFunc, n)[0].(bool) {
				return n, true
			}
			n, more = s.next()
		}
		return nil, false
	}, s.cancel}
}

func (s *stream) ChanFlatMap(mapperFunc interface{}) Stream { // <T, R> func(T) <-chan R
	var currentChan <-chan interface{}

	nextChan := func() {
		if currentChan == nil {
			n, more := s.next()
			if !more {
				return
			}
			c, cancel := generifyChannel(callFunc(mapperFunc, n)[0])
			currentChan = c
			*s.cancel = append(*s.cancel, cancel)
		}
	}

	nextItem := func() (res interface{}, retry, more bool) {
		nextChan()

		if currentChan == nil {
			return nil, false, false
		}

		next, ok := <-currentChan
		if !ok {
			currentChan = nil
			return nil, true, true
		}

		return next, false, true
	}

	return &stream{func() (interface{}, bool) {
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

func (s *stream) SliceFlatMap(mapperFunc interface{}) Stream { // <T, R> func(T) []R
	return s.ChanFlatMap(func(item interface{}) <-chan interface{} {
		if item == nil {
			return nil
		}

		slice := generifySlice(callFunc(mapperFunc, item)[0])
		resChan := make(chan interface{})

		go func() {
			for _, item := range slice {
				resChan <- item
			}
			close(resChan)
		}()

		return resChan
	})
}

func (s *stream) Take(n int) Stream {
	count := 0
	return &stream{func() (interface{}, bool) {
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

func (s *stream) Distinct() Stream {
	m := make(map[interface{}]bool)

	return &stream{func() (interface{}, bool) {
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
	return callFunc(s.compFunc, s.data[i], s.data[j])[0].(bool)
}

func (s *stream) Sort(lessFunc interface{}) Stream { // <T> func(left, right T) bool
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

	return &stream{func() (interface{}, bool) {
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

func (s *stream) WithCancel(c chan<- bool) Stream {
	cancels := append(*s.cancel, c)
	return &stream{s.next, &cancels}
}

func (s *stream) First(filterFunc interface{}) interface{} { // <T> func(T) bool
	for {
		n, more := s.next()
		if !more {
			s.finish()
			return nil
		}
		if callFunc(filterFunc, n)[0].(bool) {
			s.finish()
			return n
		}
	}
}

func (s *stream) ToSlice(t interface{}) {
	sliceValue := reflect.ValueOf(t).Elem()

	for {
		n, more := s.next()
		if !more {
			s.finish()
			return
		}
		sliceValue.Set(reflect.Append(sliceValue, reflect.ValueOf(n)))
	}
}

func (s *stream) Any(filterFunc interface{}) bool { // <T> func(T) bool
	for {
		n, more := s.next()
		if !more {
			s.finish()
			return false
		}
		if callFunc(filterFunc, n)[0].(bool) {
			s.finish()
			return true
		}
	}
}

func (s *stream) None(filterFunc interface{}) bool { // <T> func(T) bool
	return !s.Any(filterFunc)
}

func (s *stream) All(filterFunc interface{}) bool { // <T> func(T) bool
	for {
		n, more := s.next()
		if !more {
			s.finish()
			return true
		}
		if !callFunc(filterFunc, n)[0].(bool) {
			s.finish()
			return false
		}
	}
}

func (s *stream) SumInt32(mapperFunc interface{}) int32 { // <T> func(T) int32
	var res int32 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, v)[0].(int32)
	}
	s.finish()
	return res
}

func (s *stream) SumInt64(mapperFunc interface{}) int64 { // <T> func(T) int64
	var res int64 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, v)[0].(int64)
	}
	s.finish()
	return res
}

func (s *stream) SumFloat32(mapperFunc interface{}) float32 { // <T> func(T) float32
	var res float32 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, v)[0].(float32)
	}
	s.finish()
	return res
}

func (s *stream) SumFloat64(mapperFunc interface{}) float64 { // <T> func(T) float64
	var res float64 = 0
	for {
		v, more := s.next()
		if !more {
			break
		}
		res += callFunc(mapperFunc, v)[0].(float64)
	}
	s.finish()
	return res
}

func (s *stream) AvgInt(mapperFunc interface{}) int64 { // <T> func(T) int64
	var slice []interface{}
	s.ToSlice(&slice)

	var sum int64 = 0

	for _, item := range slice {
		sum += callFunc(mapperFunc, item)[0].(int64)
	}

	return int64(math.Round(float64(sum) / float64(len(slice))))
}

func (s *stream) AvgFloat(mapperFunc interface{}) float64 { // <T> func(T) float64
	var slice []interface{}
	s.ToSlice(&slice)

	var sum float64 = 0

	for _, item := range slice {
		sum += callFunc(mapperFunc, item)[0].(float64)
	}

	return sum / float64(len(slice))
}
