// Package herald is a framework
// for simplifying common server maintenance tasks.
package herald

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// Logger is an interface for logging herald status.
type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
}

// Trigger should send trigger events to herald.
type Trigger interface {
	Run(context.Context, func(map[string]interface{}))
}

const triggerExecutionDoneName = "exe_done"

// executionDone is an internal trigger activated after job finished on executor.
type executionDone struct {
	exeResult chan map[string]interface{}
}

// Run start the execution done trigger.
func (tgr *executionDone) Run(ctx context.Context, sendParam func(map[string]interface{})) {
	for {
		select {
		case <-ctx.Done():
			return
		case ep := <-tgr.exeResult:
			sendParam(ep)
		}
	}
}

// Executor will execute the job according to the param argument.
type Executor interface {
	Execute(param map[string]interface{}) map[string]interface{}
}

// Selector will decide whether jobs should be executed.
type Selector interface {
	Select(triggerParam, jogParam map[string]interface{}) bool
}

type job struct {
	param map[string]interface{}
}

type router struct {
	trigger  string
	jobs     map[string]string
	selector string
	param    map[string]interface{}
}

// Herald is the core struct.
// Do not instantiate Herald explicitly.
// Use New() function instead.
type Herald struct {
	logger    Logger
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	exeDone   chan map[string]interface{}
	triggers  map[string]Trigger
	executors map[string]Executor
	selectors map[string]Selector
	routers   map[string]*router
	jobs      map[string]*job
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

// GetTrigger will get a trigger.
// If the trigger does exist, it will return nil.
func (h *Herald) GetTrigger(name string) Trigger {
	return h.triggers[name]
}

// GetExecutor will get an executor.
// If the executor does exist, it will return nil.
func (h *Herald) GetExecutor(name string) Executor {
	return h.executors[name]
}

// GetSelector will get a selector.
// If the selector does exist, it will return nil.
func (h *Herald) GetSelector(name string) Selector {
	return h.selectors[name]
}

// RegisterTrigger will register a trigger.
// Please specify a name to use in router.
// If the name already exists, the old one will be overwritten.
func (h *Herald) RegisterTrigger(name string, tgr Trigger) error {
	if name == "" {
		return errors.New("Trigger name could not be empty")
	}
	if tgr == nil {
		return errors.New("Trigger could not be nil")
	}
	h.triggers[name] = tgr
	return nil
}

// RegisterExecutor will register an executor.
// Please specify a name to use in router.
// If the name already exists, the old one will be overwritten.
func (h *Herald) RegisterExecutor(name string, exe Executor) error {
	if name == "" {
		return errors.New("Executor name could not be empty")
	}
	if exe == nil {
		return errors.New("Executor could not be nil")
	}
	h.executors[name] = exe
	return nil
}

// RegisterSelector will register a selector.
// Please specify a name to use in router.
// If the name already exists, the old one will be overwritten.
func (h *Herald) RegisterSelector(name string, slt Selector) error {
	if name == "" {
		return errors.New("Selector name could not be empty")
	}
	if slt == nil {
		return errors.New("Selector could not be nil")
	}
	h.selectors[name] = slt
	return nil
}

// RegisterRouter will create a router.
// The router defines the rule for executing the job.
// When the trigger is activated, then try to use selector
// to check whether to execute jobs.
func (h *Herald) RegisterRouter(name, trigger, selector string, param map[string]interface{}) error {
	if selector != "" {
		_, ok := h.selectors[selector]
		if !ok {
			return fmt.Errorf("Selector does not exist : %s", selector)
		}
	}

	_, ok := h.triggers[trigger]
	if !ok {
		return fmt.Errorf("Trigger does not exist: %s", trigger)
	}

	h.routers[name] = &router{
		trigger:  trigger,
		selector: selector,
		jobs:     make(map[string]string),
		param:    deepCopyMapParam(param),
	}
	return nil
}

// AddRouterJob will add a job to the router.
// A job is assigned to an executor.
func (h *Herald) AddRouterJob(routerName, jobName, executor string) error {
	_, ok := h.routers[routerName]
	if !ok {
		return fmt.Errorf("Router does not exist : %s", routerName)
	}

	_, ok = h.executors[executor]
	if !ok {
		return fmt.Errorf("Executor does not exist: %s", executor)
	}

	h.routers[routerName].jobs[jobName] = executor
	return nil
}

// SetJobParam will set extra job specified param.
// This job param is optional.
func (h *Herald) SetJobParam(name string, param map[string]interface{}) error {
	if name == "" {
		return errors.New("Job name could not be empty")
	}
	h.jobs[name] = &job{
		param: deepCopyMapParam(param),
	}
	return nil
}

func (h *Herald) start(ctx context.Context) {
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
			tgr.Run(ctx, func(tgrParam map[string]interface{}) {
				select {
				case <-ctx.Done():
				case param <- tgrParam:
				}
			})
		}(tgr)

		triggerNames = append(triggerNames, triggerName)
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(param)})
	}

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

		h.infof("[:Herald:Trigger:%s:] Activated with ID: %s", triggerName, triggerID)

		triggerParam, ok := deepCopyParam(value.Interface()).(map[string]interface{})
		if !ok {
			h.errorf("[:Herald:] Copy trigger param error, which should not happen logically")
			continue
		}

		for routerName, r := range h.routers {
			if triggerName != r.trigger {
				continue
			}

			h.debugf(`[:Herald:Router:%s:] Trigger "%s(%s)" matched`, routerName, triggerName, triggerID)

			for jobName, executorName := range r.jobs {
				if r.selector == "" {
					h.debugf(`[:Herald:Router:%s:] Selector does not exist`, routerName)
					continue
				}

				jobParam := make(map[string]interface{})
				// Add router param to job param
				mergeMapParam(jobParam, r.param)
				// Add job specific param to job param
				jobSpecParam, ok := h.jobs[jobName]
				if ok {
					mergeMapParam(jobParam, jobSpecParam.param)
				}

				// selector
				slt, ok := h.selectors[r.selector]
				if !ok {
					h.errorf(`[:Herald:Router:%s:] Selector "%s" does not exist`, routerName, r.selector)
					continue
				}
				if !slt.Select(deepCopyMapParam(triggerParam), deepCopyMapParam(jobParam)) {
					continue
				}
				h.debugf(`[:Herald:Router:%s:] Selector "%s" accepts trigger "%s(%s)" for job "%s"`,
					routerName, r.selector, triggerName, triggerID, jobName)

				// executor
				jobID := pseudoUUID()
				exeParam := map[string]interface{}{
					"id":            jobID,
					"trigger_id":    triggerID,
					"trigger":       triggerName,
					"router":        routerName,
					"job":           jobName,
					"selector":      r.selector,
					"executor":      executorName,
					"trigger_param": deepCopyMapParam(triggerParam),
					"job_param":     deepCopyMapParam(jobParam),
				}

				h.infof(`[:Herald:Router:%s:] Execute job "%s(%s)" with executor "%s"`,
					routerName, jobName, jobID, executorName)
				h.wg.Add(1)
				go func(exe Executor) {
					defer h.wg.Done()

					result := exe.Execute(deepCopyMapParam(exeParam))
					h.infof(`[:Herald:Job:%s:] Job "%s" finished`, exeParam["job"], exeParam["id"])

					resultMap := deepCopyMapParam(exeParam)
					resultMap["result"] = result

					if h.exeDone != nil {
						select {
						case <-ctx.Done():
						case h.exeDone <- resultMap:
						}
					}
				}(h.executors[executorName])
			}
		}
	}
}

// Start the herald server.
// This function will return immediately and run in the background.
func (h *Herald) Start() {
	if h.cancel != nil {
		h.warnf("[:Herald:] Herald is already started")
		return
	}

	ctx := context.Background()
	ctx, h.cancel = context.WithCancel(ctx)

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		h.start(ctx)
	}()
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

// New will create a new Herald instance.
// The herald instance is almost empty and only include an "exe_done" trigger.
// The "exe_done" trigger will be activated after a job execution finished.
//
// A Logger interface could be passed as argument to log the herald status.
// If no output is needed, just pass nil.
func New(logger Logger) *Herald {
	h := &Herald{
		logger:    logger,
		exeDone:   make(chan map[string]interface{}),
		triggers:  make(map[string]Trigger),
		executors: make(map[string]Executor),
		selectors: make(map[string]Selector),
		routers:   make(map[string]*router),
		jobs:      make(map[string]*job),
	}
	h.RegisterTrigger(triggerExecutionDoneName, &executionDone{
		exeResult: h.exeDone,
	})
	return h
}
