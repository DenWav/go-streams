package streams_test

import (
	"github.com/DemonWav/go-streams"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
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

	require.True(t, res)
}

func TestStream(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	c := make(chan string, 2)
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

	require.True(t, res)
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
		SumInt32(func(i int) int32 {
			return int32(i)
		})

	require.EqualValues(t, 20000, sum)
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
		SumInt32(func(i int) int32 {
			return int32(i)
		})

	require.EqualValues(t, 2, sum)
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

	require.True(t, sort.IntsAreSorted(res))
}

func TestAvg(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	avg := streams.NewSliceStream(data).
		AvgInt(func(f int64) int64 {
			return f
		})

	require.EqualValues(t, 5, avg)
}

func TestCount(t *testing.T) {
	defer goleak.VerifyNoLeaks(t)

	data := []int{0, 1, 2}
	count := streams.NewSliceStream(data).
		Filter(func(i int) bool {
			return i != 1
		}).
		Count()

	require.Equal(t, 2, count)
}
