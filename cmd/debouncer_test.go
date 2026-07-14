// Copyright 1999-2026. WebPros International GmbH.

package cmd

import (
	"sync/atomic"
	"testing"
	"time"
)

func waitForCount(t *testing.T, counter *atomic.Int32, expected int32) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if counter.Load() == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("counter = %d, expected %d", counter.Load(), expected)
}

func TestDebouncerCombinesRepeatedTriggers(t *testing.T) {
	d := newDebouncer(50 * time.Millisecond)

	var calls atomic.Int32
	for range 10 {
		d.trigger("file.php", func() { calls.Add(1) })
	}

	waitForCount(t, &calls, 1)

	time.Sleep(100 * time.Millisecond)
	if got := calls.Load(); got != 1 {
		t.Errorf("calls = %d after repeated triggers, expected 1", got)
	}
}

func TestDebouncerSeparateKeys(t *testing.T) {
	d := newDebouncer(50 * time.Millisecond)

	var calls atomic.Int32
	d.trigger("first.php", func() { calls.Add(1) })
	d.trigger("second.php", func() { calls.Add(1) })

	waitForCount(t, &calls, 2)
}

func TestDebouncerTriggersAgainAfterFiring(t *testing.T) {
	d := newDebouncer(20 * time.Millisecond)

	var calls atomic.Int32
	d.trigger("file.php", func() { calls.Add(1) })
	waitForCount(t, &calls, 1)

	d.trigger("file.php", func() { calls.Add(1) })
	waitForCount(t, &calls, 2)
}
