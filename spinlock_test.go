package spinlock

import (
	"sync"
	"sync/atomic"
	"testing"
	_ "unsafe"
)

var _ sync.Locker = &Spinlock{}

func TestSpinlock(t *testing.T) {
	var (
		spin Spinlock
		wg   sync.WaitGroup
		d    = make(map[int]int)
	)
	spin.Lock()
	if ok := spin.TryLock(); ok {
		t.Fatal("should failed")
	}
	spin.Unlock()

	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			for j := 0; j < 20; j++ {
				spin.Lock()
				d[j] = i + j
				spin.Unlock()
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			spin.Lock()
			for h, m := range d {
				_ = h + m
			}
			spin.Unlock()
		}
	}()
	wg.Wait()
}

func BenchmarkSpinlock(b *testing.B) {
	var spin Spinlock
	f := func(b *testing.B, x int) {
		var N, D int64
		b.SetParallelism(x)
		b.RunParallel(func(pb *testing.PB) {
			var n, d int64
			for pb.Next() {
				b := nanotime()
				spin.Lock()
				d += nanotime() - b
				usleep(1)
				spin.Unlock()
				n++
			}
			atomic.AddInt64(&D, d)
			atomic.AddInt64(&N, n)
		})
		b.Logf("%d\t\t\t\t%d ns/op",
			atomic.LoadInt64(&N), atomic.LoadInt64(&D)/atomic.LoadInt64(&N))
	}
	b.Run("_1X", func(b *testing.B) {
		f(b, 1)
	})
	b.Run("_2X", func(b *testing.B) {
		f(b, 2)
	})
	b.Run("_4X", func(b *testing.B) {
		f(b, 4)
	})
	b.Run("_8X", func(b *testing.B) {
		f(b, 8)
	})
}

func BenchmarkSyncMutex(b *testing.B) {
	var mutex sync.Mutex
	f := func(b *testing.B, x int) {
		var N, D int64
		b.SetParallelism(x)
		b.RunParallel(func(pb *testing.PB) {
			var n, d int64
			for pb.Next() {
				b := nanotime()
				mutex.Lock()
				d += nanotime() - b
				usleep(1)
				mutex.Unlock()
				n++
			}
			atomic.AddInt64(&D, d)
			atomic.AddInt64(&N, n)
		})
		b.Logf("%d\t\t\t\t%d ns/op",
			atomic.LoadInt64(&N), atomic.LoadInt64(&D)/atomic.LoadInt64(&N))
	}
	b.Run("_1X", func(b *testing.B) {
		f(b, 1)
	})
	b.Run("_2X", func(b *testing.B) {
		f(b, 2)
	})
	b.Run("_4X", func(b *testing.B) {
		f(b, 4)
	})
	b.Run("_8X", func(b *testing.B) {
		f(b, 8)
	})
}

func BenchmarkRuntimeMutex(b *testing.B) {
	var mu mutex
	f := func(b *testing.B, x int) {
		var N, D int64
		b.SetParallelism(x)
		b.RunParallel(func(pb *testing.PB) {
			var n, d int64
			for pb.Next() {
				b := nanotime()
				lock(&mu)
				d += nanotime() - b
				usleep(1)
				unlock(&mu)
				n++
			}
			atomic.AddInt64(&D, d)
			atomic.AddInt64(&N, n)
		})
		b.Logf("%d\t\t\t\t%d ns/op",
			atomic.LoadInt64(&N), atomic.LoadInt64(&D)/atomic.LoadInt64(&N))
	}
	b.Run("_1X", func(b *testing.B) {
		f(b, 1)
	})
	b.Run("_2X", func(b *testing.B) {
		f(b, 2)
	})
	b.Run("_4X", func(b *testing.B) {
		f(b, 4)
	})
	b.Run("_8X", func(b *testing.B) {
		f(b, 8)
	})
}

func BenchmarkSpinlock_TryLock(b *testing.B) {
	var spin Spinlock
	f := func(b *testing.B, x int) {
		b.SetParallelism(x)
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if spin.TryLock() {
					usleep(1)
					spin.Unlock()
				}
			}
		})
	}
	b.Run("_1X", func(b *testing.B) {
		f(b, 1)
	})
	b.Run("_2X", func(b *testing.B) {
		f(b, 2)
	})
	b.Run("_4X", func(b *testing.B) {
		f(b, 4)
	})
	b.Run("_8X", func(b *testing.B) {
		f(b, 8)
	})
}

func BenchmarkYield(b *testing.B) {
	b.Run("procyield(10)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			procyield(10)
		}
	})
	b.Run("procyield(20)", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			procyield(20)
		}
	})
	b.Run("osyield()", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			osyield()
		}
	})
}

//go:linkname usleep runtime.usleep
func usleep(usec uint32)

//go:linkname nanotime runtime.nanotime
func nanotime() int64

//go:linkname unlock runtime.unlock
func unlock(l *mutex)

//go:linkname lock runtime.lock
func lock(l *mutex)

type mutex struct {
	key uintptr
}
