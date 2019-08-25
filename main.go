package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"reflect"
	"sync"
	"syscall"
	"time"
)

// Trigger should send trigger events to the core
type Trigger interface {
	Start(context.Context, chan string)
}

// Runner will run jobs
type Runner interface {
	Execute(string)
}

type job struct {
	trigger Trigger
	runner  Runner
	param   chan string
}

type herald struct {
	wg   sync.WaitGroup
	jobs map[string]*job
}

func (h *herald) addJob(name string, j *job) {
	h.jobs[name] = j
}

func (h *herald) start(ctx context.Context) {
	go func() {
		h.wg.Add(1)
		defer h.wg.Done()

		var names []string
		var cases []reflect.SelectCase

		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})

		for name, j := range h.jobs {
			j.param = make(chan string)

			log.Printf("Start trigger %s...\n", name)

			go func(j *job) {
				h.wg.Add(1)
				defer h.wg.Done()
				j.trigger.Start(ctx, j.param)
			}(j)

			names = append(names, name)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(j.param)})
		}

		log.Println("Waiting for triggers...")

		for {
			chosen, value, _ := reflect.Select(cases)

			if chosen == 0 {
				log.Println("Stop herald...")
				return
			}

			name := names[chosen-1]

			log.Printf("Start to execute %s...\n", name)

			go func() {
				h.wg.Add(1)
				defer h.wg.Done()
				h.jobs[name].runner.Execute(value.Interface().(string))
			}()
		}
	}()
}

func (h *herald) wait() {
	h.wg.Wait()
}

func main() {
	h := &herald{
		jobs: make(map[string]*job),
	}

	j := &job{
		trigger: &triggerTick{
			interval: 2 * time.Second,
		},
		runner: &runnerLocal{},
	}
	h.addJob("first", j)

	j2 := &job{
		trigger: &triggerTick{
			interval: 3 * time.Second,
		},
		runner: &runnerLocal{},
	}
	h.addJob("second", j2)

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	h.start(ctx)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown...")

	cancel()

	h.wait()
	log.Println("Exiting...")
}
