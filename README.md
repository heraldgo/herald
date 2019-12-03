# Herald

[![GoDoc](https://godoc.org/github.com/heraldgo/herald?status.svg)](https://godoc.org/github.com/heraldgo/herald)

Herald is a library written in [Go](https://golang.org/)
for simplifying ordinary server maintenance tasks.

In case you need a ready-to-use program, try the
[Herald Daemon](https://github.com/heraldgo/heraldd)
which is based on Herald.

Herald is not designed to do massive works.
It is suitable for jobs like daily backup, automatically program deployment,
and other repetitive server maintenance tasks.


## Components

Herald consists of the following components:

* Trigger
* Selector
* Executor
* Router
* Job

The core logic for herald is simple.
The routers define when (trigger, selector) and
how (executor) to execute certain jobs.
When a specific trigger is actived, then the selector will check
whether it is OK to execute subsequent jobs.

Herald does not provide implementation for trigger, selector and executor.
They are defined as interfaces and should be provided in your application.
[Herald Daemon](https://github.com/heraldgo/heraldd) has already
defined some useful components.


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
It includes how to write trigger, executor and selector,
also how to setup the herald workflow.

This example will be activated every 2 seconds and print the job param.
Press `Ctrl+C` to exit.

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

func (slt *all) Select(triggerParam, jobParam map[string]interface{}) bool {
	return true
}

func newHerald() *herald.Herald {
	h := herald.New(nil)

	h.RegisterTrigger("tick", &tick{
		interval: 2 * time.Second,
	})
	h.RegisterExecutor("print", &printParam{})
	h.RegisterSelector("all", &all{})

	h.RegisterRouter("tick_test", "tick", "all", nil)
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

A full example could also be installed by `go get -u github.com/heraldgo/herald/herald-example`
and then run `herald-example`.


## Logging

The `New()` function accept an `Logger` interface as argument.
Here is a simple implementation.

```go
import (
	"log"
)

type simpleLogger struct{}

// Debugf is ignored
func (l *simpleLogger) Debugf(f string, v ...interface{}) {}

func (l *simpleLogger) Infof(f string, v ...interface{}) {
	log.Printf("[INFO] "+f, v...)
}

func (l *simpleLogger) Warnf(f string, v ...interface{}) {
	log.Printf("[WARN] "+f, v...)
}

func (l *simpleLogger) Errorf(f string, v ...interface{}) {
	log.Printf("[ERROR] "+f, v...)
}

func main() {
	h := herald.New(&simpleLogger{})
	...
}
```

If [logrus](https://github.com/sirupsen/logrus) is preferred,
`*logrus.Logger` is natively an implementation of `Logger` interface.

```go
import (
	"github.com/sirupsen/logrus"
)

func main() {
	h := herald.New(logrus.New())
	...
}
```

The logger could also be shared between `Herald` and your application.

```go
import (
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	h := herald.New(logger)

	logger.Info("Start to run herald")
	h.Start()
	...
}
```


## Trigger

The trigger will run in the background and should send activation signal
under certain conditions.

The trigger is defined as an interface:

```go
type Trigger interface {
	Run(context.Context, func(map[string]interface{}))
}
```

This is an example of trigger which will be activated periodically:

```go
import (
	"time"
)

type tick struct {
	interval time.Duration
}

func (tgr *tick) Run(ctx context.Context, sendParam func(map[string]interface{})) {
	ticker := time.NewTicker(tgr.interval)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			counter++
			sendParam(map[string]interface{}{"counter": counter})
		}
	}
}
```

The `Run` function must be implemented in the trigger. The `Run` function
will keep running in the background after `Herald.Start`.
It should deal with `ctx.Done()` properly in order to exit gracefully,
or the program may be blocked when trying to stop.

A map param could be sent to herald when it is activated. This param
will be used as "trigger param" and passed to selector and executor.
The param should be a json-like object,
so it is flexible to put complex data in it. Since this param will be
passed to another goroutine, it is better to send a new param variable each time.

Register the trigger in herald with a name.
Each trigger has a name which will be used as an identifier in router.
The name must be all different for triggers. Registering a same name will
overwrite the old one.

```go
h.RegisterTrigger("tick", &tick{
	interval: 2 * time.Second,
})
```

There could be many different triggers registered in herald. It is not
recommended to register the same trigger instance with different names.
In this case you must do it with great care in the trigger because
they will run in different goroutines.
It is better to create a new instance for the new trigger name.

> `exe_done` is the only predefined trigger which will be activated when an
> execution is done. The result of the executor is used as "trigger param",
> which could be passed to selector and executor.
> Do NOT register your trigger with name `exe_done`, which will overwrite the
> internal one.


## Selector

A selector will filter out from the "trigger param" to determine whether
or not to run the following jobs.

An example of selector which will only accept the even activation of the
tick trigger.

```go
type even struct{}

func (slt *even) Select(triggerParam, jobParam map[string]interface{}) bool {
	if triggerParam["counter"].(int) % 2 == 0 {
		return true
	}
	return false
}
```

> Here ignores type assertion error for the param.
> You may need more checks in order not to panic.

The `Select` function must be implemented in the selector.
`Select` function accept "trigger param" and "job param" as
arguments. "trigger param" is passed from the trigger and "job
param" is the combination of the "router param" and "job specific param".
The returned boolean value determines whether to proceed.


## Executor

An executor will execute the job according to "param".

An example of executor which will just print the param.

```go
type printParam struct{}

func (exe *printParam) Execute(param map[string]interface{}) map[string]interface{} {
	log.Printf("Execute with param: %v", param)
	return nil
}
```

The `Execute` function must be implemented in the executor.
"executor param" includes details of the job:

```
id: F60CFC6A-2FDE-248D-6C35-C3EFD484014F
info:
  router: router_name
  trigger: trigger_name
  trigger_id: A8D875BC-5875-3BA7-EECB-F829A341F78E
  selector: selector_name
  job: job_name
trigger_param: map[string]interface{}
job_param: map[string]interface{}
```

The returned map value of `Execute` will be used as the
"trigger param" of internal `exe_done` trigger.

> Each job will be executed in a separated goroutine. Try not to modify
> any variables outside the function for safety reason.


## Router

The routers define when (trigger, selector) and
how (executor) to execute certain jobs.
One router includes a trigger, a selector, jobs and params.

This is what a router looks like:

```
trigger: trigger_name
selector: selector_name
job:
  job1_name: executor1_name
  job2_name: executor1_name
  job3_name: executor2_name
router_param:
  key1: value1
  key2: value2
```

Register a router to herald:

```go
h.RegisterRouter("router_name", "trigger_name", "selector_name", routerParam)
```

Add jobs to the router and specify the executor:

```go
h.AddRouterJob("router_name", "job1_name", "executor1_name")
h.AddRouterJob("router_name", "job2_name", "executor1_name")
h.AddRouterJob("router_name", "job3_name", "executor2_name")
```

The job names in the same router must be all different.


## Job

A job defines the job param which will be passed to an selector or executor.
It is created when executing `AddRouterJob`:

```go
h.AddRouterJob("router_name", "job_name", "executor_name")
```

The "job param" is the combination of the "router param" and "job specific param".
In case there are conflicts, the "job specific param" has higher
priority.

The "job specific param" is optional. It is set by:

```go
h.SetJobParam("job_name", jobSpecParam)
```

The same job name could be used among different routers, which will
share the same "job specific param".
