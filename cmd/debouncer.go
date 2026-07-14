package cmd

import (
	"sync"
	"time"
)

type debouncer struct {
	delay  time.Duration
	mu     sync.Mutex
	timers map[string]*time.Timer
}

func newDebouncer(delay time.Duration) *debouncer {
	return &debouncer{
		delay:  delay,
		timers: make(map[string]*time.Timer),
	}
}

func (d *debouncer) trigger(key string, fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if timer, ok := d.timers[key]; ok {
		timer.Reset(d.delay)
		return
	}

	d.timers[key] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, key)
		d.mu.Unlock()

		fn()
	})
}
