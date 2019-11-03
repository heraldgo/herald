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
	exeParam chan map[string]interface{}
}

// Run start the execution done trigger
func (tgr *executionDone) Run(ctx context.Context, param chan map[string]interface{}) {
	for {
		select {
		case <-ctx.Done():
			return
		case ep := <-tgr.exeParam:
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
		param:    deepCopyMapParam(param),
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
		param: deepCopyMapParam(param),
	}
}

func (h *Herald) start(ctx context.Context) {
	defer h.wg.Done()

	cases := make([]reflect.SelectCase, 0, len(h.triggers)+1)

	cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(ctx.Done())})

	triggerNames := make([]string, 0, len(h.triggers))

	triggerChanStartIndex := len(cases)
	for triggerName, tgr := range h.triggers {
		param := make(chan map[string]interface{})

		h.infof("[:Herald:] Start trigger %s...", triggerName)

		h.wg.Add(1)
		go func(tgr Trigger) {
			defer h.wg.Done()
			tgr.Run(ctx, param)
		}(tgr)

		triggerNames = append(triggerNames, triggerName)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
	}

	h.infof("[:Herald:] Start to wait for triggers activation...")

	for {
		chosen, value, _ := reflect.Select(cases)

		// ctx.Done()
		if chosen == 0 {
			h.infof("[:Herald:] Stop herald...")
			return
		}

		// triggers
		triggerName := triggerNames[chosen-triggerChanStartIndex]
		triggerID := pseudoUUID()

		h.infof("[:Trigger:%s:] Activated with ID: %s", triggerName, triggerID)

		triggerParam, ok := deepCopyParam(value.Interface()).(map[string]interface{})
		if !ok {
			h.errorf("[:Herald:] Copy trigger param error, which should not happen logically")
			continue
		}

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

			h.infof(`[:Router:%s:] Trigger "%s(%s)" matched`, routerName, triggerName, triggerID)

			for jobName, executors := range r.jobs {
				jobParam := make(map[string]interface{})

				// Add router param to job param
				mergeMapParam(jobParam, r.param)

				// Add job specific param to job param
				jobSpecParam, ok := h.jobs[jobName]
				if ok {
					mergeMapParam(jobParam, jobSpecParam.param)
				}

				var filterParam map[string]interface{}
				if r.filter != "" {
					flt, ok := h.filters[r.filter]
					if !ok {
						h.errorf(`[:Router:%s:] Filter "%s" not found`, routerName, r.filter)
						continue
					}
					filterParam, ok = flt.Filter(deepCopyMapParam(triggerParam), deepCopyMapParam(jobParam))
					h.infof(`[:Router:%s:] Filter "%s" tests trigger "%s(%s)" for job "%s" passed: %t`,
						routerName, r.filter, triggerName, triggerID, jobName, ok)
					if !ok {
						continue
					}
				} else {
					// If no filter provided, just pass the full trigger param to executor
					filterParam = triggerParam
				}

				for _, executorName := range executors {
					jobID := pseudoUUID()
					exeParam := make(map[string]interface{})
					exeParam["id"] = jobID
					exeParam["info"] = map[string]interface{}{
						"trigger_id": triggerID,
						"job":        jobName,
						"router":     routerName,
						"trigger":    triggerName,
						"filter":     r.filter,
						"executor":   executorName,
					}
					exeParam["filter_param"] = deepCopyMapParam(filterParam)
					exeParam["job_param"] = deepCopyMapParam(jobParam)

					h.infof(`[:Router:%s:] Execute job "%s(%s)" with executor "%s"`,
						routerName, jobName, jobID, executorName)
					h.wg.Add(1)
					go func(exe Executor, param map[string]interface{}) {
						defer h.wg.Done()

						result := exe.Execute(deepCopyMapParam(param))

						if param["info"].(map[string]interface{})["trigger"] != triggerExecutionDoneName {
							resultMap := deepCopyMapParam(param)
							mergeMapParam(resultMap, result)
							h.exeDone <- resultMap
						}
					}(h.executors[executorName], exeParam)
				}
			}
		}
	}
}

// Start the herald server
func (h *Herald) Start() {
	ctx := context.Background()
	ctx, h.cancel = context.WithCancel(ctx)

	h.wg.Add(1)
	go h.start(ctx)
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
		exeParam: h.exeDone,
	})
	return h
}
