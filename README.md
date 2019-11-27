# Herald

Herald is a library written in [Go](https://golang.org/)
for simplifying common server maintenance tasks.

In case you need a ready-to-use program, try the
[Herald Daemon](https://github.com/heraldgo/heraldd)
which is based on Herald.


## Components

Herald consists of the following components:

* Trigger
* Selector
* Transformer
* Executor
* Router
* Job


### Trigger
### Selector
### Transformer
### Executor


## Installation

First install [Go](https://golang.org/) and setup the workspace,
then use the following command to install `Herald`.

```shell
$ go get -u github.com/heraldgo/herald
```

Import it in the code:

```go
import "github.com/heraldgo/herald"
```


## Example

Here is a simple example which shows how to write a herald program.

```go
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heraldgo/herald"
)

// tick triggers periodically
type tick struct {
	interval time.Duration
}

func (tgr *tick) Run(ctx context.Context, sendParam func(map[string]interface{})) {
	ticker := time.NewTicker(tgr.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sendParam(nil)
		}
	}
}

// print executor just print the param
type printParam struct{}

func (exe *printParam) Execute(param map[string]interface{}) map[string]interface{} {
	log.Printf("[Executor(Print)] Execute with param: %v", param)
	return nil
}

// all selector pass all conditions
type all struct{}

func (slt *all) Select(triggerParam, selectorParam map[string]interface{}) bool {
	return true
}

func newHerald() *herald.Herald {
	h := herald.New(nil)

	h.AddTrigger("tick", &tick{
		interval: 2 * time.Second,
	})
	h.AddExecutor("print", &printParam{})
	h.AddSelector("all", &all{})

	h.AddRouter("tick_test", "tick", "all", "", nil)
	h.AddRouterJob("tick_test", "print_it", "print")

	return h
}

func main() {
	log.Printf("Initialize...")
	h := newHerald()
	log.Printf("Start...")

	h.Start()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Printf("Shutdown...")
	h.Stop()
	log.Printf("Exit...")
}
```
A full example could also be installed by `go get -u github.com/heraldgo/herald/herald-example`.
