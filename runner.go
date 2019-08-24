package main

import (
	"log"
)

type runnerLocal struct {
}

func (r *runnerLocal) Start(param string) {
	log.Printf("Run with param: %s\n", param)
}
