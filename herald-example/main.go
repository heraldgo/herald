package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/heraldgo/herald"
)

type simpleLogger struct{}

// Debugf is a simple implementation
func (l *simpleLogger) Debugf(f string, v ...interface{}) {}

// Infof is a simple implementation
func (l *simpleLogger) Infof(f string, v ...interface{}) {
	log.Printf("[INFO] "+f, v...)
}

// Warnf is a simple implementation
func (l *simpleLogger) Warnf(f string, v ...interface{}) {
	log.Printf("[WARN] "+f, v...)
}

// Errorf is a simple implementation
func (l *simpleLogger) Errorf(f string, v ...interface{}) {
	log.Printf("[ERROR] "+f, v...)
}

var logger herald.Logger

// tick trigger
type tick struct {
	interval time.Duration
}

func (tgr *tick) Run(ctx context.Context, sendParam func(map[string]interface{})) {
	ticker := time.NewTicker(tgr.interval)
	defer ticker.Stop()

	counter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			counter++
			sendParam(map[string]interface{}{"counter": counter})
		}
	}
}

// print param executor
type printParam struct {
	logger herald.Logger
}

func (exe *printParam) Execute(param map[string]interface{}) map[string]interface{} {
	exe.logger.Infof("[Executor(Print)] Execute with param: %v", param)
	return nil
}

// skip filter to skip certain numbers
type skip struct{}

func (flt *skip) Filter(triggerParam, filterParam map[string]interface{}) bool {
	skipNumber, ok := filterParam["skip_number"].(int)
	if !ok || skipNumber <= 0 {
		return true
	}

	counter, ok := triggerParam["counter"].(int)
	if !ok || counter%(skipNumber+1) != 0 {
		return false
	}

	return true
}

func newHerald() *herald.Herald {
	h := herald.New(logger)

	h.AddTrigger("tick", &tick{
		interval: 3 * time.Second,
	})

	h.AddExecutor("print", &printParam{
		logger: logger,
	})

	h.AddFilter("skip", &skip{})

	h.AddRouter("skip_test", "tick", "skip", "", map[string]interface{}{
		"skip_number": 2,
	})

	h.AddRouterJob("skip_test", "print", "print")

	return h
}

func main() {
	logger = &simpleLogger{}

	logger.Infof("Initialize...")

	h := newHerald()

	logger.Infof("Start...")

	h.Start()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	logger.Infof("Shutdown...")

	h.Stop()

	logger.Infof("Exit...")
}
