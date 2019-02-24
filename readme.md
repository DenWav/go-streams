go-streams
==========

[![Build Status](https://ci.demonwav.com/app/rest/builds/buildType:(id:GoStreams_Build)/statusIcon)](https://ci.demonwav.com/viewType.html?buildTypeId=GoStreams_Build) [![GoDoc](https://godoc.org/github.com/DemonWav/go-streams?status.svg)](https://godoc.org/github.com/DemonWav/go-streams) ![Current Release](https://img.shields.io/badge/release-v1.0.0-orange.svg?style=flat-square)

`go-streams` is a library which provides a stream mechanism based on Java 8's Stream API. A Stream is a lazily evaluated
chain of functions which operates on some source of values. Streams allows you to define a pipeline of operations to
perform on a source of iterated values. The pieces of the pipeline are lazily evaluated, so for example, items which
fail a Filter operation will not be passed to a following Map operation (or any operation).

This library does not depend on any other packages at runtime - all dependencies are for tests only.

Use this library with:

```go
import "github.com/DemonWav/go-streams"
```

A typical usage of this API might look something like this:

```go
package main

import (
        "fmt"
        "github.com/DemonWav/go-streams"
)

func main() {
        cancel := make(chan bool, 1)
        c := make(chan int)

        go func() {
                for i := 2; ; i++ {
                        select {
                        case c <- i:
                        case <-cancel:
                                close(c)
                                return
                        }
                }
        }()

        seen := make([]int, 0)

        streams.NewChanStream(c).
                WithCancel(cancel).
                Filter(func(i int) bool {
                        return streams.NewSliceStream(seen).None(func(n int) bool {
                                return i%n == 0
                        })
                }).
                OnEach(func(i int) {
                        seen = append(seen, i)
                }).
                Take(100).
                ForEach(func(i int) {
                        fmt.Println(i)
                })
}
```

In this example, an infinite Stream of integers is used as a channel source for the Stream, and the first 100 prime
numbers are printed using the Sieve of Eratosthenes. The number of total prime numbers output can be controlled simply
by modifying the argument to the `Take` method.

[Read the docs for in-depth information on how to use this library.](https://godoc.org/github.com/DemonWav/go-streams)
