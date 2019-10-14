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

// Trigger should send trigger events to the core
type Trigger interface {
	Run(context.Context, chan map[string]interface{})
}

const triggerExecutionDoneName = "exe_done"

type executionDone struct {
	logger   Logger
	exeParam chan map[string]interface{}
}

// Run start the execution done trigger
func (tgr *executionDone) Run(ctx context.Context, param chan map[string]interface{}) {
	for {
		select {
		case <-ctx.Done():
			tgr.logger.Infof("[Trigger(ExecutionDone)] Exit")
			return
		case ep := <-tgr.exeParam:
			tgr.logger.Infof("[Trigger(ExecutionDone)] Previous execution finished")
			param <- ep
		}
	}
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
	logger    Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	exeDone   chan map[string]interface{}
	triggers  map[string]Trigger
	executors map[string]Executor
	filters   map[string]Filter
	jobs      map[string]*job
	routers   map[string]*router
}

func (h *Herald) debugf(f string, v ...interface{}) {
	if h.logger != nil {
		h.logger.Debugf(f, v...)
	}
}

func (h *Herald) infof(f string, v ...interface{}) {
	if h.logger != nil {
		h.logger.Infof(f, v...)
	}
}

func (h *Herald) warnf(f string, v ...interface{}) {
	if h.logger != nil {
		h.logger.Warnf(f, v...)
	}
}

func (h *Herald) errorf(f string, v ...interface{}) {
	if h.logger != nil {
		h.logger.Errorf(f, v...)
	}
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
			h.errorf("[:Herald:] Filter not found : %s", filter)
			return
		}
	}

	var availTriggers []string
	for _, tgr := range triggers {
		_, ok := h.triggers[tgr]
		if !ok {
			h.errorf("[:Herald:] Trigger not found and will be ignored: %s", tgr)
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
		h.errorf("[:Herald:] Router not found : %s", routerName)
		return
	}

	var availExecutors []string
	for _, exe := range executors {
		_, ok := h.executors[exe]
		if !ok {
			h.errorf("[:Herald:] Executor not found and will be ignored: %s", exe)
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

	var cases []reflect.SelectCase

	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})

	var triggerNames []string
	triggerChanStartIndex := len(cases)
	for triggerName, tgr := range h.triggers {
		param := make(chan map[string]interface{})

		h.infof("[:Herald:] Start trigger %s...", triggerName)

		go func(tgr Trigger) {
			h.wg.Add(1)
			defer h.wg.Done()
			tgr.Run(ctx, param)
		}(tgr)

		triggerNames = append(triggerNames, triggerName)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
	}

	h.infof("[:Herald:] Start to wait for triggers activation...")

	for {
		chosen, value, _ := reflect.Select(cases)

		if chosen == 0 {
			h.infof("[:Herald:] Stop herald...")
			return
		}

		triggerName := triggerNames[chosen-triggerChanStartIndex]

		h.infof("[:Trigger:%s:] Activated", triggerName)

		triggerParam := value.Interface().(map[string]interface{})

		for routerName, r := range h.routers {
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

			h.infof("[:Router:%s:] Trigger \"%s\" matched", routerName, triggerName)

			for jobName, executors := range r.jobs {
				exeParam := make(map[string]interface{})
				for k, v := range r.param {
					exeParam[k] = v
				}
				_, ok := h.jobs[jobName]
				if ok {
					for k, v := range h.jobs[jobName].param {
						exeParam[k] = v
					}
				}

				var filterParam map[string]interface{}
				if r.filter != "" {
					filterParam, ok = h.filters[r.filter].Filter(triggerParam, exeParam)

					passed := "OK"
					if !ok {
						passed = "Failed"
					}

					h.infof("[:Router:%s:] Filter \"%s\" tests trigger \"%s\" for job \"%s\": %s", routerName, r.filter, triggerName, jobName, passed)
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
					h.infof("[:Router:%s:] Execute job \"%s\" with executor \"%s\"", routerName, jobName, executorName)
					go func(exe Executor, param map[string]interface{}) {
						h.wg.Add(1)
						defer h.wg.Done()
						result := exe.Execute(param)
						h.exeDone <- result
					}(h.executors[executorName], exeParam)
				}
			}
		}
	}
}

// Stop will stop the server and wait for all triggers and executors to exit
func (h *Herald) Stop() {
	if h.cancel == nil {
		h.warnf("[:Herald:] Herald is not started")
		return
	}

	h.cancel()
	h.cancel = nil

	h.wg.Wait()
}

// New will create a new empty Herald instance
func New(logger Logger) *Herald {
	h := &Herald{
		logger:    logger,
		exeDone:   make(chan map[string]interface{}),
		triggers:  make(map[string]Trigger),
		executors: make(map[string]Executor),
		filters:   make(map[string]Filter),
		jobs:      make(map[string]*job),
		routers:   make(map[string]*router),
	}
	h.AddTrigger(triggerExecutionDoneName, &executionDone{
		logger:   logger,
		exeParam: h.exeDone,
	})
	return h
}
