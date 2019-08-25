package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	h := New()

	h.addTrigger("Trigger2s", &triggerTick{
		interval: 2 * time.Second,
	})

	h.addTrigger("Trigger3s", &triggerTick{
		interval: 3 * time.Second,
	})

	h.addRunner("RunnerPrint", &runnerLocal{})

	h.addJob("Print2s", "Trigger2s", "RunnerPrint")
	h.addJob("Print3s", "Trigger3s", "RunnerPrint")

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	h.start(ctx)

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown...")

	cancel()

	h.wait()
	log.Println("Exiting...")
}
