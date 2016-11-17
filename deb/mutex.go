package deb

import (
	"time"
	"sync"
)

// TryLocker extends Locker interface with TryLock
type TryLocker interface {
	sync.Locker
	TryLock(timeout time.Duration) bool
}

// Mutex implements mutex with channels and a timeout
// possibility
type Mutex struct {
	c chan struct{}
}

// NewMutex creates a new empty mutex
func NewMutex() *Mutex {
	return &Mutex{make(chan struct{}, 1)}
}

// TryLockMutexes tries to lock all mutexes if one fails it will return will
// false and cleans up locks
func TryLockMutexes(timeout time.Duration, mutexes ...TryLocker) bool {
	var lockedMutexes []TryLocker
	for _, mu := range mutexes {
		ok := mu.TryLock(timeout)
		if !ok {
			// clean up locks
			for _, lmu := range lockedMutexes {
				lmu.Unlock()
			}
			return false
		}

		lockedMutexes = append(lockedMutexes, mu)
	}

	return true
}

// Lock locks mutex
func (m *Mutex) Lock() {
	m.c <- struct{}{}
}

// Unlock unlocks mutex
func (m *Mutex) Unlock() {
	<-m.c
}

// TryLock tries to lock with a wait timeout
func (m *Mutex) TryLock(timeout time.Duration) bool {
	// TODO configure timeout in aptly configuration
	timer := time.NewTimer(timeout)
	select {
	case m.c <- struct{}{}:
		timer.Stop()
		return true
	case <-time.After(timeout):
	}
	return false
}
