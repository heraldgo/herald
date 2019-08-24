package main

import (
	"fmt"
)

// Trigger should send trigger events to the core
type Trigger interface {
	Init()
	Run()
}

// Runner will run jobs
type Runner interface {
	Run()
}

func main() {
	fmt.Println("vim-go")
}
