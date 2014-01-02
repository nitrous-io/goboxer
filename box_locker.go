package goboxer

import (
	"sync"
)

type lockable struct {
	lock   sync.Mutex
	locked bool
}

func (l *lockable) Lock() {
	l.locked = true
	l.lock.Lock()
}

func (l *lockable) Unlock() {
	l.lock.Unlock()
	l.locked = false
}

type BoxLocker map[int]*lockable

func (b BoxLocker) Lock(boxId int) {
	var locker *lockable
	locker, ok := b[boxId]
	if !ok {
		b[boxId] = &lockable{}
		locker = b[boxId]
	}
	locker.Lock()
}

func (b BoxLocker) Unlock(boxId int) {
	locker, ok := b[boxId]
	if ok {
		locker.Unlock()
	}
}
