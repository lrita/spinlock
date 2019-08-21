// Package spinlock implements the MCS-Spinlock by
// https://www.cs.rochester.edu/u/scott/papers/1991_TOCS_synch.pdf

package spinlock

import (
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

// due to https://github.com/golang/go/issues/14620, in some situation, we
// cannot make the object aligned by composited.
type issues14620 struct {
	_ *qnode
}

type qnodeInternal struct {
	lock uintptr
	_    [7]uintptr
	next unsafe.Pointer
}

type qnode struct {
	qnodeInternal
	// Prevents false sharing on widespread platforms with
	// 128 mod (cache line size) = 0.
	_ [128 - unsafe.Sizeof(qnodeInternal{})%128]byte
}

type Spinlock struct {
	next  unsafe.Pointer
	_     [7]unsafe.Pointer
	held  unsafe.Pointer
	_     [7]unsafe.Pointer
	qn    unsafe.Pointer
	qsize uintptr
	lock  sync.Mutex
}

var null = unsafe.Pointer(&qnode{})

func (l *Spinlock) qnode() *qnode {
	pid := runtime_procPin() // lock m to prevent from swap-out
	qsize := atomic.LoadUintptr(&l.qsize)
	qn := atomic.LoadPointer(&l.qn)
	if uintptr(pid) >= qsize {
		runtime_procUnpin()
		l.lock.Lock()
		pid = runtime_procPin()
		qsize = atomic.LoadUintptr(&l.qsize)
		qn = atomic.LoadPointer(&l.qn)
		if uintptr(pid) >= qsize {
			qsize = uintptr(runtime.GOMAXPROCS(0))
			n := make([]qnode, qsize)
			qn = unsafe.Pointer(&n[0])
			atomic.StorePointer(&l.qn, qn)       // store-release
			atomic.StoreUintptr(&l.qsize, qsize) // store-release
		}
		l.lock.Unlock()
	}

	return (*qnode)(unsafe.Pointer(uintptr(qn) +
		uintptr(pid)*unsafe.Sizeof(qnode{})))
}

func (l *Spinlock) Lock() {
	n := l.qnode()
	old := (*qnode)(atomic.SwapPointer(&l.next, unsafe.Pointer(n)))
	if old == nil {
		atomic.StorePointer(&l.held, unsafe.Pointer(n))
		return // got
	}
	atomic.StoreUintptr(&n.lock, 1)
	atomic.StorePointer(&old.next, unsafe.Pointer(n))

	var x, spin uint32
	if runtime.NumCPU() > 1 {
		spin = 0x04
	}
	for atomic.LoadUintptr(&n.lock) == 1 {
		if x&spin == spin {
			osyield() // 50ns schedule once
		} else {
			procyield(10)
		}
		x++
	}

	atomic.StorePointer(&l.held, unsafe.Pointer(n))
}

// TryLock returns true if we are holding this spinlock.
func (l *Spinlock) TryLock() bool {
	n := l.qnode()
	ok := atomic.CompareAndSwapPointer(&l.next, nil, unsafe.Pointer(n))
	if ok {
		atomic.StorePointer(&l.held, unsafe.Pointer(n))
	} else {
		runtime_procUnpin()
	}
	return ok
}

func (l *Spinlock) Unlock() {
	n := (*qnode)(atomic.LoadPointer(&l.held))
	next := (*qnode)(atomic.LoadPointer(&n.next))
	if next == nil {
		if atomic.CompareAndSwapPointer(&l.next, unsafe.Pointer(n), nil) {
			runtime_procUnpin()
			return
		}
		var x, spin uint32
		if runtime.NumCPU() > 1 {
			spin = 0x04
		}
		for {
			if next = (*qnode)(atomic.LoadPointer(&n.next)); next != nil {
				break
			}
			if x&spin == spin {
				osyield() // 50ns schedule once
			} else {
				procyield(10)
			}
			x++
		}
	}
	// pass to next
	atomic.StoreUintptr(&next.lock, 0)
	n.next = nil
	runtime_procUnpin()
}

// Implemented in runtime.

//go:linkname runtime_procPin runtime.procPin
//go:nosplit
func runtime_procPin() int

//go:linkname runtime_procUnpin runtime.procUnpin
//go:nosplit
func runtime_procUnpin()

//go:linkname procyield runtime.procyield
func procyield(cycles uint32)

//go:linkname osyield runtime.osyield
func osyield()
