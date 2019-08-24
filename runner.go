package main

import (
	"log"
)

type runnerLocal struct {
}

func (r *runnerLocal) Execute(param string) {
	log.Printf("Execute with param: %s\n", param)
}
