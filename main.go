package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Trigger should send trigger events to the core
type Trigger interface {
	Init()
	Start()
}

// Runner will run jobs
type Runner interface {
	Run()
}

type herald struct {
	param    chan string
	stopChan chan struct{}
	quitChan chan struct{}
	triggers map[string]Trigger
	runners  map[string]Runner
}

func (h *herald) addTrigger(t Trigger) {
}

func (h *herald) addRunner(r Runner) {
}

func (h *herald) start() {
	go func() {
		h.stopChan = make(chan struct{})
		h.quitChan = make(chan struct{})

		defer func() {
			close(h.quitChan)
		}()

		for {
			select {
			case <-h.stopChan:
				log.Println("Stop herald...")
				return
			default:
				log.Println("In herald...")
				time.Sleep(2 * time.Second)
			}
		}
	}()
}

func (h *herald) stop() {
	close(h.stopChan)

	select {
	case <-h.quitChan:
		log.Println("Wait to stop...")
	}
}

func main() {
	h := &herald{}

	h.addTrigger(&triggerCron{})
	h.addRunner(&runnerLocal{})

	h.start()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	log.Println("Shutdown...")

	h.stop()

	log.Println("Exiting...")
}
