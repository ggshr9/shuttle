package engine

import (
	"sync"
	"testing"
	"time"

	"github.com/ggshr9/shuttle/config"
)

func TestEmit_ConcurrentUnsubscribeNoDeadlock(t *testing.T) {
	e := New(config.DefaultClientConfig())

	done := make(chan struct{})
	go func() {
		defer close(done)

		ch := e.Subscribe()

		// Drain events so the channel doesn't fill up.
		go func() {
			for range ch {
			}
		}()

		// Emit many events while unsubscribing concurrently.
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				e.emit(Event{Type: EventSpeedTick})
			}
		}()

		go func() {
			defer wg.Done()
			// Let some events flow before unsubscribing.
			time.Sleep(time.Millisecond)
			e.Unsubscribe(ch)
		}()

		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected: test did not complete within 5s")
	}
}

func TestEmit_ConcurrentSubscribeUnsubscribe(t *testing.T) {
	e := New(config.DefaultClientConfig())

	done := make(chan struct{})
	go func() {
		defer close(done)

		var wg sync.WaitGroup

		// Emitter goroutine.
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10000; i++ {
				e.emit(Event{Type: EventSpeedTick})
			}
		}()

		// 100 goroutines each subscribe and unsubscribe.
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ch := e.Subscribe()
				// Drain so sends don't block.
				go func() {
					for range ch {
					}
				}()
				time.Sleep(time.Millisecond)
				e.Unsubscribe(ch)
			}()
		}

		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected: test did not complete within 5s")
	}
}
