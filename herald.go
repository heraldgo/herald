package main

import (
	"context"
	"log"
	"reflect"
	"sync"
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
	trigger string
	runner  string
}

// Herald is the core struct
type Herald struct {
	wg       sync.WaitGroup
	triggers map[string]Trigger
	runners  map[string]Runner
	jobs     map[string]*job
}

func (h *Herald) addTrigger(name string, t Trigger) {
	h.triggers[name] = t
}

func (h *Herald) addRunner(name string, r Runner) {
	h.runners[name] = r
}

func (h *Herald) addJob(name, trigger, runner string) {
	h.jobs[name] = &job{
		trigger: trigger,
		runner:  runner,
	}
}

func (h *Herald) start(ctx context.Context) {
	go func() {
		h.wg.Add(1)
		defer h.wg.Done()

		var cases []reflect.SelectCase

		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})

		var triggerNames []string
		triggerChanStart := len(cases)
		for triggerName, t := range h.triggers {
			param := make(chan string)

			log.Printf("Start trigger %s...\n", triggerName)

			go func(t Trigger) {
				h.wg.Add(1)
				defer h.wg.Done()
				t.Start(ctx, param)
			}(t)

			triggerNames = append(triggerNames, triggerName)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
		}

		log.Println("Waiting for triggers...")

		for {
			chosen, value, _ := reflect.Select(cases)

			if chosen == 0 {
				log.Println("Stop herald...")
				return
			}

			triggerName := triggerNames[chosen-triggerChanStart]

			log.Printf("Trigger active: \"%s\"\n", triggerName)

			var runnerNames []string

			for _, j := range h.jobs {
				if j.trigger == triggerName {
					runnerNames = append(runnerNames, j.runner)
				}
			}

			for _, runnerName := range runnerNames {
				log.Printf("Job execute with runner \"%s\"\n", runnerName)
				go func(r Runner) {
					h.wg.Add(1)
					defer h.wg.Done()
					r.Execute(value.Interface().(string))
				}(h.runners[runnerName])
			}
		}
	}()
}

func (h *Herald) wait() {
	h.wg.Wait()
}

// New will create a new Herald instance
func New() *Herald {
	return &Herald{
		triggers: make(map[string]Trigger),
		runners:  make(map[string]Runner),
		jobs:     make(map[string]*job),
	}
}
