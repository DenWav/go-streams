package streams_test

import (
	"github.com/DemonWav/go-streams"
	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
	"math"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"unicode"
)

func TestStream_All(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	s := []string{"hello", "world"}

	res := streams.NewSliceStream(s).
		Filter(func(word string) bool {
			return strings.Contains(word, "or")
		}).
		Map(func(word string) string {
			return strings.ToUpper(word)
		}).
		SliceFlatMap(func(word string) []rune {
			return []rune(word)
		}).
		None(func(char rune) bool {
			return unicode.IsSpace(char)
		})

	assert.True(t, res)
}

func TestStream(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	c := make(chan string)
	go func() {
		c <- "hello"
		c <- " asdf ad fas "
		c <- " asdf asd as"
		c <- "world"
		close(c)
	}()

	res := streams.NewChanStream(c).
		Filter(func(word string) bool {
			return strings.Contains(word, "el")
		}).
		Map(func(word string) string {
			return strings.ToUpper(word)
		}).
		ChanFlatMap(func(word string) <-chan rune {
			c := make(chan rune)
			go func() {
				for _, r := range word {
					c <- r
				}
				close(c)
			}()
			return c
		}).
		None(func(char rune) bool {
			return unicode.IsSpace(char)
		})

	assert.True(t, res)
}

func Test(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	cancel := make(chan bool, 1)
	c := make(chan int)

	go func() {
		for i := 0; ; i++ {
			select {
			case c <- i:
			case <-cancel:
				close(c)
				return
			}
		}
	}()

	sum := streams.NewChanStream(c).
		WithCancel(cancel).
		Map(func(i int) int {
			return i * 2
		}).
		Filter(func(i int) bool {
			return i%4 != 0
		}).
		Take(100).
		SumInt(func(i int) int64 {
			return int64(i)
		})

	assert.EqualValues(t, 20000, sum)
}

func TestDistinct(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	cancel := make(chan bool, 1)
	c := make(chan int)

	go func() {
		for {
			select {
			case c <- 1:
			case <-cancel:
				close(c)
				return
			}
		}
	}()

	sum := streams.NewChanStream(c, cancel).
		Map(func(i int) int {
			return i * 2
		}).
		Take(100).
		Distinct().
		SumInt(func(i int) int64 {
			return int64(i)
		})

	assert.EqualValues(t, 2, sum)
}

func TestSort(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	cancel := make(chan bool, 1)
	c := make(chan int)

	go func() {
		for {
			select {
			case c <- rand.Int():
			case <-cancel:
				close(c)
				return
			}
		}
	}()

	var res []int
	streams.NewChanStream(c, cancel).
		Take(10000).
		Sort(func(left, right int) bool {
			return left < right
		}).
		ToSlice(&res)

	assert.True(t, sort.IntsAreSorted(res))
}

func TestAvg(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	avg := streams.NewSliceStream(data).
		AvgInt(func(f int64) int64 {
			return f
		})

	assert.EqualValues(t, 5, avg)
}

func TestCount(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	count := streams.NewSliceStream(data).
		Filter(func(i int) bool {
			return i != 1
		}).
		Count()

	assert.Equal(t, 2, count)
}

func TestForEach(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	var output []int

	streams.NewSliceStream(data).
		ForEach(func(i int) {
			output = append(output, i)
		})

	assert.Equal(t, data, output)
}

func TestToChan(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	var output []int

	c := make(chan int)
	q := make(chan bool, 1)

	go func() {
		for {
			select {
			case i, ok := <-c:
				if !ok {
					return
				}
				output = append(output, i)
			case <-q:
				return
			}
		}
	}()

	streams.NewSliceStream(data).
		WithCancel(q).
		ToChan(c)

	assert.Equal(t, data, output)
}

func TestSkip(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	expected := []int{2}
	var output []int

	streams.NewSliceStream(data).
		Skip(2).
		ToSlice(&output)

	assert.Equal(t, expected, output)
}

func TestOnEach(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	var output1 []int
	var output2 []int

	streams.NewSliceStream(data).
		OnEach(func(i int) {
			output2 = append(output2, i)
		}).
		ForEach(func(i int) {
			output1 = append(output1, i)
		})

	assert.Equal(t, data, output1)
	assert.Equal(t, data, output2)
}

func TestMin(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{1, 6, -5}

	var val int

	streams.NewSliceStream(data).
		Min(&val, func(left, right int) bool {
			return left < right
		})

	assert.Equal(t, -5, val)
}

func TestMax(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{1, 6, -5}

	var val int

	streams.NewSliceStream(data).
		Max(&val, func(left, right int) bool {
			return left < right
		})

	assert.Equal(t, 6, val)
}

func TestReduce(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := "The quick brown fox jumped over the lazy sheep dog."

	var res map[rune]int

	streams.NewSliceStream([]string{data}).
		SliceFlatMap(func(line string) []rune {
			return []rune(line)
		}).
		Filter(func(char rune) bool {
			return unicode.IsLetter(char)
		}).
		Map(func(char rune) rune {
			return unicode.ToLower(char)
		}).
		Reduce(&res, make(map[rune]int), func(char rune, m map[rune]int) map[rune]int {
			m[char]++
			return m
		})

	// The expected value hilariously shows how in-applicable this problem is to the above solution :D
	exp := make(map[rune]int)
	for _, c := range data {
		if !unicode.IsLetter(c) {
			continue
		}
		exp[unicode.ToLower(c)]++
	}

	assert.Equal(t, exp, res)
}

func TestReduceMin(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{1, 6, -5}

	var valMin int
	var valReduce int

	streams.NewSliceStream(data).
		Min(&valMin, func(left, right int) bool {
			return left < right
		})

	streams.NewSliceStream(data).
		Reduce(&valReduce, int(math.MaxInt32), func(item, out int) int {
			if item < out {
				return item
			} else {
				return out
			}
		})

	assert.Equal(t, valMin, valReduce)
}

func TestConcat(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}

	var out []int

	streams.NewSliceStream(data).
		Concat(streams.NewSliceStream(data)).
		Concat(streams.NewSliceStream(data)).
		ToSlice(&out)

	var expected []int
	expected = append(expected, data...)
	expected = append(expected, data...)
	expected = append(expected, data...)

	assert.Equal(t, expected, out)
}

func TestZip(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}

	var out []int

	streams.NewSliceStream(data).
		Zip(streams.NewSliceStream(data), 0, func(left, right int) int {
			return left + right
		}).
		ToSlice(&out)

	expected := make([]int, len(data))
	for i, v := range data {
		expected[i] = v + v
	}

	assert.Equal(t, expected, out)
}

func TestMismatchedZip(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}

	var out []int

	streams.NewSliceStream(data).
		Concat(streams.NewSliceStream(data)).
		Zip(streams.NewSliceStream(data), 0, func(left, right int) int {
			return left + right
		}).
		ToSlice(&out)

	expected := make([]int, len(data)*2)
	for i, v := range data {
		expected[i] = v + v
	}
	for i, v := range data {
		expected[len(data)+i] = v
	}

	assert.Equal(t, expected, out)
}
