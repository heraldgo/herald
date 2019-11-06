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
	Run(context.Context, func(map[string]interface{}))
}

const triggerExecutionDoneName = "exe_done"

// executionDone is an internal trigger activated after job finished on executor
type executionDone struct {
	exeResult chan map[string]interface{}
}

// Run start the execution done trigger
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

// Executor will do the execution
type Executor interface {
	Execute(param map[string]interface{}) map[string]interface{}
}

// Filter will filter trigger to check whether to execute jobs
type Filter interface {
	Filter(triggerParam, filterParam map[string]interface{}) bool
}

// Transformer will transform trigger param
type Transformer interface {
	Transform(triggerParam map[string]interface{}) map[string]interface{}
}

type job struct {
	param map[string]interface{}
}

type router struct {
	trigger     string
	filter      string
	transformer string
	jobs        map[string]string
	param       map[string]interface{}
}

// Herald is the core struct
type Herald struct {
	logger       Logger
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	exeDone      chan map[string]interface{}
	triggers     map[string]Trigger
	transformers map[string]Transformer
	executors    map[string]Executor
	filters      map[string]Filter
	jobs         map[string]*job
	routers      map[string]*router
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

// GetTrigger will get a trigger
func (h *Herald) GetTrigger(name string) (Trigger, bool) {
	tgr, ok := h.triggers[name]
	return tgr, ok
}

// GetExecutor will get a executor
func (h *Herald) GetExecutor(name string) (Executor, bool) {
	exe, ok := h.executors[name]
	return exe, ok
}

// GetFilter will get a filter
func (h *Herald) GetFilter(name string) (Filter, bool) {
	flt, ok := h.filters[name]
	return flt, ok
}

// GetTransformer will get a filter
func (h *Herald) GetTransformer(name string) (Transformer, bool) {
	tfm, ok := h.transformers[name]
	return tfm, ok
}

// AddTrigger will add a trigger
func (h *Herald) AddTrigger(name string, tgr Trigger) {
	h.triggers[name] = tgr
}

// AddTransformer will add a transformer
func (h *Herald) AddTransformer(name string, tfm Transformer) {
	h.transformers[name] = tfm
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
func (h *Herald) AddRouter(name, trigger, filter, transformer string, param map[string]interface{}) {
	if filter != "" {
		_, ok := h.filters[filter]
		if !ok {
			h.errorf("[:Herald:] Filter not found : %s", filter)
			return
		}
	}

	if transformer != "" {
		_, ok := h.transformers[transformer]
		if !ok {
			h.errorf("[:Herald:] Transformer not found : %s", transformer)
			return
		}
	}

	_, ok := h.triggers[trigger]
	if !ok {
		h.errorf("[:Herald:] Trigger not found: %s", trigger)
		return
	}

	h.routers[name] = &router{
		trigger:     trigger,
		filter:      filter,
		transformer: transformer,
		jobs:        make(map[string]string),
		param:       deepCopyMapParam(param),
	}
}

// AddRouterJob will add executor and param
func (h *Herald) AddRouterJob(routerName, jobName, executor string) {
	_, ok := h.routers[routerName]
	if !ok {
		h.errorf("[:Herald:] Router not found : %s", routerName)
		return
	}

	_, ok = h.executors[executor]
	if !ok {
		h.errorf("[:Herald:] Executor not found: %s", executor)
		return
	}

	h.routers[routerName].jobs[jobName] = executor
}

// SetJobParam will set job specified param
func (h *Herald) SetJobParam(name string, param map[string]interface{}) {
	h.jobs[name] = &job{
		param: deepCopyMapParam(param),
	}
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

			h.infof(`[:Herald:Router:%s:] Trigger "%s(%s)" matched`, routerName, triggerName, triggerID)

			// transformer
			finalTriggerParam := triggerParam
			if r.transformer != "" {
				tfm, ok := h.transformers[r.transformer]
				if !ok {
					h.errorf(`[:Herald:Router:%s:] Transformer "%s" not found`, routerName, r.transformer)
					continue
				} else {
					finalTriggerParam = tfm.Transform(deepCopyMapParam(triggerParam))
					h.infof(`[:Herald:Router:%s:] Trigger param transformed by "%s"`, routerName, r.transformer)
				}
			}

			for jobName, executorName := range r.jobs {
				if r.filter == "" {
					h.debugf(`[:Herald:Router:%s:] Filter not found`, routerName)
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

				// filter
				flt, ok := h.filters[r.filter]
				if !ok {
					h.errorf(`[:Herald:Router:%s:] Filter "%s" not found`, routerName, r.filter)
					continue
				}
				passed := flt.Filter(deepCopyMapParam(triggerParam), deepCopyMapParam(jobParam))
				if !passed {
					continue
				}
				h.infof(`[:Herald:Router:%s:] Filter "%s" tests trigger "%s(%s)" for job "%s" passed`,
					routerName, r.filter, triggerName, triggerID, jobName)

				// executor
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
				exeParam["trigger_param"] = deepCopyMapParam(finalTriggerParam)
				exeParam["job_param"] = deepCopyMapParam(jobParam)

				h.infof(`[:Herald:Router:%s:] Execute job "%s(%s)" with executor "%s"`,
					routerName, jobName, jobID, executorName)
				h.wg.Add(1)
				go func(exe Executor) {
					defer h.wg.Done()

					result := exe.Execute(deepCopyMapParam(exeParam))

					resultMap := deepCopyMapParam(exeParam)
					mergeMapParam(resultMap, result)

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

// Start the herald server
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

// New will create a new Herald instance
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
		exeResult: h.exeDone,
	})
	return h
}
