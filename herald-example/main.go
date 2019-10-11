package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xianghuzhao/herald"
)

type loggerSimple struct{}

// Debugf is a simple implementation
func (l *loggerSimple) Debugf(f string, v ...interface{}) {
	log.Printf("[DEBUG] "+f, v...)
}

// Infof is a simple implementation
func (l *loggerSimple) Infof(f string, v ...interface{}) {
	log.Printf("[INFO] "+f, v...)
}

// Warnf is a simple implementation
func (l *loggerSimple) Warnf(f string, v ...interface{}) {
	log.Printf("[WARN] "+f, v...)
}

// Errorf is a simple implementation
func (l *loggerSimple) Errorf(f string, v ...interface{}) {
	log.Printf("[ERROR] "+f, v...)
}

var logger herald.Logger

type tick struct {
	interval time.Duration
	counter  int
}

func (tgr *tick) Run(ctx context.Context, param chan map[string]interface{}) {
	ticker := time.NewTicker(tgr.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			tgr.counter++
			param <- map[string]interface{}{"counter": tgr.counter}
		}
	}
}

type printParam struct {
	logger herald.Logger
}

func (exe *printParam) Execute(param map[string]interface{}) map[string]interface{} {
	exe.logger.Infof("[Executor(Print)] Execute with param: %#v", param)
	return nil
}

type skip struct{}

func (flt *skip) Filter(triggerParam, filterParam map[string]interface{}) (map[string]interface{}, bool) {
	skipNumber, ok := filterParam["skip_number"]
	if !ok {
		return triggerParam, true
	}
	skipNumberInt, ok := skipNumber.(int)
	if !ok || skipNumberInt <= 0 {
		return triggerParam, true
	}

	counter, ok := triggerParam["counter"]
	if !ok {
		return nil, false
	}
	counterInt, ok := counter.(int)
	if !ok || counterInt%(skipNumberInt+1) != 0 {
		return nil, false
	}

	return triggerParam, true
}

func newHerald() *herald.Herald {
	h := herald.New()

	h.Log = logger

	h.AddTrigger("tick", &tick{
		interval: 3 * time.Second,
	})

	h.AddExecutor("print", &printParam{
		logger: logger,
	})

	h.AddFilter("skip", &skip{})

	h.AddRouter("skip_test", []string{"tick"}, "skip", map[string]interface{}{
		"skip_number": 2,
	})

	h.AddRouterJob("skip_test", "print", []string{"print"})

	return h
}

func main() {
	logger = &loggerSimple{}

	h := newHerald()

	go h.Start()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	logger.Infof("Shutdown...")

	h.Stop()

	logger.Infof("Exiting...")
}
