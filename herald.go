package herald

import (
	"context"
	"reflect"
	"sync"
)

// Logger is an interface for common log
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
}

type emptyLogger struct{}

// Debugf is an empty implementation
func (l *emptyLogger) Debugf(string, ...interface{}) {}

// Infof is an empty implementation
func (l *emptyLogger) Infof(string, ...interface{}) {}

// Warnf is an empty implementation
func (l *emptyLogger) Warnf(string, ...interface{}) {}

// Errorf is an empty implementation
func (l *emptyLogger) Errorf(string, ...interface{}) {}

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
	Log       Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	triggers  map[string]Trigger
	executors map[string]Executor
	filters   map[string]Filter
	jobs      map[string]*job
	routers   map[string]*router
}

// GetTrigger will add a trigger
func (h *Herald) GetTrigger(name string) (Trigger, bool) {
	tgr, ok := h.triggers[name]
	return tgr, ok
}

// GetExecutor will add a executor
func (h *Herald) GetExecutor(name string) (Executor, bool) {
	exe, ok := h.executors[name]
	return exe, ok
}

// GetFilter will add a filter
func (h *Herald) GetFilter(name string) (Filter, bool) {
	flt, ok := h.filters[name]
	return flt, ok
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
			h.Log.Errorf("Filter not found : %s", filter)
			return
		}
	}

	var availTriggers []string
	for _, tgr := range triggers {
		_, ok := h.triggers[tgr]
		if !ok {
			h.Log.Errorf("Trigger not found and will be ignored: %s", tgr)
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
		h.Log.Errorf("Router not found : %s", routerName)
		return
	}

	var availExecutors []string
	for _, exe := range executors {
		_, ok := h.executors[exe]
		if !ok {
			h.Log.Errorf("Executor not found and will be ignored: %s", exe)
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

		h.Log.Infof("Start trigger %s...", triggerName)

		go func(tgr Trigger) {
			h.wg.Add(1)
			defer h.wg.Done()
			tgr.Run(ctx, param)
		}(tgr)

		triggerNames = append(triggerNames, triggerName)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
	}

	h.Log.Infof("Start to wait for triggers activation...")

	for {
		chosen, value, _ := reflect.Select(cases)

		if chosen == 0 {
			h.Log.Infof("Stop herald...")
			return
		}

		var triggerName string

		if chosen == 1 {
			h.Log.Infof("Execution finished ...")
			triggerName = "execution_done"
		} else {
			triggerName = triggerNames[chosen-triggerChanStartIndex]
		}

		h.Log.Infof("Trigger active: \"%s\"", triggerName)

		triggerParam := value.Interface().(map[string]interface{})

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

				var filterParam map[string]interface{}
				if r.filter != "" {
					filterParam, ok = h.filters[r.filter].Filter(triggerParam, exeParam)
					if !ok {
						continue
					}
				} else {
					filterParam = triggerParam
				}
				for k, v := range filterParam {
					exeParam[k] = v
				}

				for _, executorName := range executors {
					h.Log.Infof("Execute job \"%s\" with executor: %s", j, executorName)
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
		h.Log.Warnf("Herald is not started")
		return
	}

	h.cancel()
	h.cancel = nil

	h.wg.Wait()
}

// New will create a new empty Herald instance
func New() *Herald {
	return &Herald{
		Log:       &emptyLogger{},
		triggers:  make(map[string]Trigger),
		executors: make(map[string]Executor),
		filters:   make(map[string]Filter),
		jobs:      make(map[string]*job),
		routers:   make(map[string]*router),
	}
}
