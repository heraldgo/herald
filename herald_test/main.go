package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/xianghuzhao/herald"
	"github.com/xianghuzhao/herald/herald/executor"
	"github.com/xianghuzhao/herald/herald/filter"
	"github.com/xianghuzhao/herald/herald/trigger"
)

func newHerald() *herald.Herald {
	h := herald.New()

	h.AddTrigger("tick", &trigger.Tick{
		Interval: 2 * time.Second,
	})

	h.AddExecutor("print", &executor.Print{})

	h.AddFilter("skip", &filter.Skip{})

	h.AddRouter("skip_test", []string{"tick"}, "skip", map[string]interface{}{
		"skip_number": "jfdk",
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
