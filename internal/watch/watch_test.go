package watch

import (
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

const timeout = 2 * time.Second

func TestValueStress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const (
			setCount   = 1000
			watchCount = 50
		)

		v := NewValue(int(0))
		var watches [watchCount]Watch

		var handlerGroup sync.WaitGroup
		handlerGroup.Add(watchCount)
		for i := range watchCount {
			var sum int
			var sawFinal bool
			watches[i] = v.Watch(func(x int) {
				sum += x // Shouldn't race, as watches only run one handler at a time.
				switch {
				case sawFinal && x < setCount:
					assert.Fail(t, "Previous state shouldn't be handled after final state")
				case sawFinal && x == setCount:
					assert.Fail(t, "Final state shouldn't be seen more than once")
				case !sawFinal && x == setCount:
					handlerGroup.Done()
					sawFinal = true
				}
			})
		}

		// Set should be concurrency-safe.
		var setGroup sync.WaitGroup
		for i := 1; i <= setCount-1; i++ {
			setGroup.Go(func() { v.Set(i) })
		}

		// When write A happens-before write B, any handler execution for A should
		// happen-before the handler execution for B (verified in the handler).
		setGroup.Wait()
		v.Set(setCount)
		handlerGroup.Wait()

		// Termination shouldn't deadlock (verified by synctest).
		for _, w := range watches {
			w.Cancel()
			w.Wait()
		}
	})
}

func TestGetZeroValue(t *testing.T) {
	assert.Nil(t, new(Value[any]).Get())
}

func TestWatchZeroValue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notify := make(chan any)
		w := new(Value[any]).Watch(func(x any) { notify <- x })
		assert.Nil(t, <-notify)
		w.Cancel()
		w.Wait()
	})
}

func TestSetBlockedWatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := NewValue("alice")

		// The blocked watch has an extra channel compared to the unblocked watch,
		// which lets us assert that a handler is in flight.
		block, notifyBlocked := make(chan struct{}), make(chan string)
		blockedWatcher := v.Watch(func(x string) {
			<-block
			notifyBlocked <- x
		})

		notifyUnblocked := make(chan string)
		unblockedWatcher := v.Watch(func(x string) { notifyUnblocked <- x })

		// Both watches should handle the initial value normally.
		block <- struct{}{}
		assert.Equal(t, "alice", <-notifyBlocked)
		assert.Equal(t, "alice", <-notifyUnblocked)

		// Set a new value, and ensure the blocked watch's handler is in flight
		// before continuing.
		v.Set("bob")
		block <- struct{}{}

		// Blockage of one watch shouldn't block the other.
		assert.Equal(t, "bob", <-notifyUnblocked)
		v.Set("carol")
		assert.Equal(t, "carol", <-notifyUnblocked)
		v.Set("eve")
		assert.Equal(t, "eve", <-notifyUnblocked)

		// Finish the blocked watch's handler for "bob".
		assert.Equal(t, "bob", <-notifyBlocked)

		// The blocked watch should run a handler for "eve", which was set while
		// it was blocked.
		close(block)
		assert.Equal(t, "eve", <-notifyBlocked)

		// Termination shouldn't deadlock (verified by synctest).
		blockedWatcher.Cancel()
		blockedWatcher.Wait()
		unblockedWatcher.Cancel()
		unblockedWatcher.Wait()
	})
}

func TestSetFromHandler(t *testing.T) {
	// As a special case of a blocked watch, a handler can Set its own Value.
	synctest.Test(t, func(t *testing.T) {
		const stopValue = 10

		v := NewValue(int(0))
		done := make(chan struct{})
		w := v.Watch(func(x int) {
			if x >= stopValue {
				close(done)
			} else {
				v.Set(x + 1)
				v.Set(x + 1)
			}
		})

		<-done
		assert.Equal(t, stopValue, v.Get())
		w.Cancel()
		w.Wait()
	})
}

func TestGoexitFromHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := NewValue("alice")
		notify := make(chan string)
		w := v.Watch(func(x string) {
			notify <- x
			runtime.Goexit()
		})

		// The initial value should get sent before the handler Goexits.
		assert.Equal(t, "alice", <-notify)

		// The first handler's Goexit shouldn't break the watch.
		v.Set("bob")
		assert.Equal(t, "bob", <-notify)

		// Termination shouldn't deadlock (verified by synctest).
		w.Cancel()
		w.Wait()
	})
}

func TestCancelInactiveHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notify := make(chan string)
		v := NewValue("alice")
		w := v.Watch(func(x string) { notify <- x })

		// Ensure the initial handler is finished, then cancel the watch.
		assert.Equal(t, "alice", <-notify)
		synctest.Wait()
		w.Cancel()

		// The canceled watch shouldn't trigger a new notification.
		v.Set("bob")
		synctest.Wait()
		select {
		case <-notify:
			assert.Fail(t, "Handler shouldn't run after cancellation")
		default:
		}
	})
}

func TestDoubleCancelInactiveHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := NewValue("alice")
		w := v.Watch(func(x string) {})

		// Ensure the initial handler is finished.
		synctest.Wait()

		// Canceling more than once shouldn't panic or deadlock.
		w.Cancel()
		w.Cancel()
		w.Wait()
	})
}

func TestCancelBlockedWatcher(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := NewValue("alice")

		block, notify := make(chan struct{}), make(chan string)
		w := v.Watch(func(x string) {
			<-block
			notify <- x
		})

		// Ensure the initial handler is in flight.
		block <- struct{}{}

		// Set some new values. If the watch weren't about to be canceled, it would
		// need to run a handler for "carol".
		v.Set("bob")
		v.Set("carol")

		// Cancel the watch. Now, it must _not_ run a handler for "carol".
		w.Cancel()

		// Even with yet another value, the watch must not run a new handler.
		v.Set("eve")

		// Let the initial handler finish.
		assert.Equal(t, "alice", <-notify)

		// Termination shouldn't deadlock (verified by synctest thanks to the
		// handler's blocking channel operations).
		w.Wait()
	})
}

func TestDoubleCancelFromHandler(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		v := NewValue("alice")

		var canceled bool
		watchCh := make(chan Watch)
		w := v.Watch(func(x string) {
			if !assert.False(t, canceled, "Handler shouldn't run after cancellation") {
				return
			}

			// Set a new value while the handler (this code!) is in flight. If the
			// watch weren't about to be canceled, it would need to run a handler
			// for "bob".
			v.Set("bob")

			// Double-cancel the watch with the handler in flight, which shouldn't
			// panic or deadlock. Now, it must _not_ run a handler for "bob".
			// The handler shouldn't have trouble canceling its own watch.
			w := <-watchCh
			w.Cancel()
			w.Cancel()
			canceled = true
		})

		// Termination shouldn't deadlock (verified by synctest).
		watchCh <- w
		w.Wait()

		// The new value should still have been set.
		assert.Equal(t, "bob", v.Get())
	})
}

func TestWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		notify := make(chan string)
		v := NewValue("alice")
		w := v.Watch(func(x string) { notify <- x })

		// Ensure the initial handler is in flight.
		synctest.Wait()

		// Start a Wait call in the background.
		var done atomic.Bool
		go func() { defer done.Store(true); w.Wait() }()

		// Wait should be blocked.
		synctest.Wait()
		assert.False(t, done.Load(), "Watch shouldn't finish immediately")

		// Set a new value and ensure we have a new handler for "bob". Wait should
		// still be blocked.
		v.Set("bob")
		assert.Equal(t, "alice", <-notify)
		synctest.Wait()
		assert.False(t, done.Load(), "Watch shouldn't finish before cancellation")

		// Cancel the watch. Wait should still be blocked.
		w.Cancel()
		synctest.Wait()
		assert.False(t, done.Load(), "Watch shouldn't finish before handler exit")

		// Let the handler finish. Wait should finally return.
		assert.Equal(t, "bob", <-notify)
		synctest.Wait()
		assert.True(t, done.Load(), "Watch should terminate")
	})
}

func BenchmarkSet1Watcher(b *testing.B) {
	benchmarkSetWithWatchers(b, 1)
}

func BenchmarkSet10Watchers(b *testing.B) {
	benchmarkSetWithWatchers(b, 10)
}

func BenchmarkSet100Watchers(b *testing.B) {
	benchmarkSetWithWatchers(b, 100)
}

func BenchmarkSet1000Watchers(b *testing.B) {
	benchmarkSetWithWatchers(b, 1000)
}

func benchmarkSetWithWatchers(b *testing.B, nWatchers int) {
	v := NewValue(uint64(0))
	watchers := make([]Watch, nWatchers)
	for i := range watchers {
		var sum uint64
		watchers[i] = v.Watch(func(x uint64) { sum += x })
	}

	b.Cleanup(func() {
		for _, w := range watchers {
			w.Cancel()
		}
		for _, w := range watchers {
			w.Wait()
		}
	})

	b.RunParallel(func(pb *testing.PB) {
		// Why random values, specifically? First, making the values unpredictable
		// is more realistic. Second, the purpose of the benchmark is to compare
		// watcher implementations, so the RNG's work basically factors out.
		for pb.Next() {
			v.Set(rand.Uint64())
		}
	})
}
