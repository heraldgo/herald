package main

import (
	"context"
	"strconv"
	"time"
)

type triggerTick struct {
	interval time.Duration
	counter  int
}

func (t *triggerTick) Start(ctx context.Context, param chan string) {
	ticker := time.NewTicker(t.interval)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.counter++
			param <- strconv.Itoa(t.counter)
		}
	}
}
