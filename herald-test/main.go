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

type logger struct{}

// Debugf is an empty implementation
func (l *logger) Debugf(f string, v ...interface{}) {
	log.Printf(f, v...)
}

// Infof is an empty implementation
func (l *logger) Infof(f string, v ...interface{}) {
	log.Printf(f, v...)
}

// Warnf is an empty implementation
func (l *logger) Warnf(f string, v ...interface{}) {
	log.Printf(f, v...)
}

// Errorf is an empty implementation
func (l *logger) Errorf(f string, v ...interface{}) {
	log.Printf(f, v...)
}

type tick struct {
	Interval time.Duration
	counter  int
}

func (tgr *tick) Run(ctx context.Context, param chan map[string]interface{}) {
	ticker := time.NewTicker(tgr.Interval)
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
	Log herald.Logger
}

func (exe *printParam) Execute(param map[string]interface{}) map[string]interface{} {
	exe.Log.Infof("[Executor:Print] Execute with param:\n%#v\n", param)
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

	h.Log = &logger{}

	h.AddTrigger("tick", &tick{
		Interval: 2 * time.Second,
	})

	h.AddExecutor("print", &printParam{
		Log: h.Log,
	})

	h.AddFilter("skip", &skip{})

	h.AddRouter("skip_test", []string{"tick"}, "skip", map[string]interface{}{
		"skip_number": 3,
	})

	h.AddRouterJob("skip_test", "print", []string{"print"})

	return h
}

func main() {
	h := newHerald()

	go h.Start()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown...")

	h.Stop()

	log.Println("Exiting...")
}
