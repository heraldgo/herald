package herald

import (
	"context"
	"log"
	"reflect"
	"sync"
)

// Trigger should send trigger events to the core
type Trigger interface {
	Run(context.Context, chan map[string]interface{})
}

// Executor will do the execution
type Executor interface {
	Execute(param map[string]interface{}) map[string]interface{}
}

// Filter triggers for jobs
type Filter interface {
	Filter(triggerParam, filterParam map[string]interface{}) (map[string]interface{}, bool)
}

type job struct {
	param map[string]interface{}
}

type router struct {
	triggers []string
	filter   string
	jobs     map[string][]string
	param    map[string]interface{}
}

// Herald is the core struct
type Herald struct {
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	triggers  map[string]Trigger
	executors map[string]Executor
	filters   map[string]Filter
	jobs      map[string]*job
	routers   map[string]*router
}

// AddTrigger will add a trigger
func (h *Herald) AddTrigger(name string, tgr Trigger) {
	h.triggers[name] = tgr
}

// AddExecutor will add a executor
func (h *Herald) AddExecutor(name string, exe Executor) {
	h.executors[name] = exe
}

// AddFilter will add a filter
func (h *Herald) AddFilter(name string, flt Filter) {
	h.filters[name] = flt
}

// AddRouter will add executor and param
func (h *Herald) AddRouter(name string, triggers []string, filter string, param map[string]interface{}) {
	if filter != "" {
		_, ok := h.filters[filter]
		if !ok {
			log.Printf("Filter not found : %s\n", filter)
			return
		}
	}

	var availTriggers []string
	for _, tgr := range triggers {
		_, ok := h.triggers[tgr]
		if !ok {
			log.Printf("Trigger not found and will be ignored: %s\n", tgr)
			continue
		}
		availTriggers = append(availTriggers, tgr)
	}

	h.routers[name] = &router{
		triggers: availTriggers,
		filter:   filter,
		jobs:     make(map[string][]string),
		param:    param,
	}
}

// AddRouterJob will add executor and param
func (h *Herald) AddRouterJob(routerName, jobName string, executors []string) {
	_, ok := h.routers[routerName]
	if !ok {
		log.Printf("Router not found : %s\n", routerName)
		return
	}

	var availExecutors []string
	for _, exe := range executors {
		_, ok := h.executors[exe]
		if !ok {
			log.Printf("Executor not found and will be ignored: %s\n", exe)
			continue
		}
		availExecutors = append(availExecutors, exe)
	}

	h.routers[routerName].jobs[jobName] = availExecutors
}

// SetJobParam will add job specified param
func (h *Herald) SetJobParam(name string, param map[string]interface{}) {
	h.jobs[name] = &job{
		param: param,
	}
}

// Start the herald server
func (h *Herald) Start() {
	h.wg.Add(1)
	defer h.wg.Done()

	ctx := context.Background()
	ctx, h.cancel = context.WithCancel(ctx)

	executionDone := make(chan map[string]interface{})

	var cases []reflect.SelectCase

	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})
	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(executionDone)})

	var triggerNames []string
	triggerChanStartIndex := len(cases)
	for triggerName, tgr := range h.triggers {
		param := make(chan map[string]interface{})

		log.Printf("Start trigger %s...\n", triggerName)

		go func(tgr Trigger) {
			h.wg.Add(1)
			defer h.wg.Done()
			tgr.Run(ctx, param)
		}(tgr)

		triggerNames = append(triggerNames, triggerName)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
	}

	log.Println("Start to wait for triggers activation...")

	for {
		chosen, value, _ := reflect.Select(cases)

		if chosen == 0 {
			log.Println("Stop herald...")
			return
		}

		var triggerName string

		if chosen == 1 {
			log.Println("Execution finished ...")
			triggerName = "execution_done"
		} else {
			triggerName = triggerNames[chosen-triggerChanStartIndex]
		}

		log.Printf("Trigger active: \"%s\"\n", triggerName)

		triggerValue := value.Interface().(map[string]interface{})

		for _, r := range h.routers {
			triggerMatched := false
			for _, t := range r.triggers {
				if t == triggerName {
					triggerMatched = true
					break
				}
			}
			if !triggerMatched {
				continue
			}

			for j, executors := range r.jobs {
				exeParam := make(map[string]interface{})
				for k, v := range r.param {
					exeParam[k] = v
				}
				_, ok := h.jobs[j]
				if ok {
					for k, v := range h.jobs[j].param {
						exeParam[k] = v
					}
				}

				if r.filter != "" {
					exeParam, ok = h.filters[r.filter].Filter(triggerValue, exeParam)
					if !ok {
						continue
					}
				}

				for _, executorName := range executors {
					log.Printf("Execute job \"%s\" with executor: %s\n", j, executorName)
					go func(exe Executor, param map[string]interface{}) {
						h.wg.Add(1)
						defer h.wg.Done()
						result := exe.Execute(param)
						executionDone <- result
					}(h.executors[executorName], exeParam)
				}
			}
		}
	}
}

// Stop will stop the server and wait for all triggers and executors to exit
func (h *Herald) Stop() {
	if h.cancel == nil {
		log.Printf("Herald is not started")
		return
	}

	h.cancel()
	h.cancel = nil

	h.wg.Wait()
}

// New will create a new empty Herald instance
func New() *Herald {
	return &Herald{
		triggers:  make(map[string]Trigger),
		executors: make(map[string]Executor),
		filters:   make(map[string]Filter),
		jobs:      make(map[string]*job),
		routers:   make(map[string]*router),
	}
}
