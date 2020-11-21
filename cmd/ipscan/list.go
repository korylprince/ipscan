package main

import (
	"encoding/binary"
	"sort"
	"sync"
)

type List struct {
	list []*Host
	mu   *sync.RWMutex
	err  error
}

func NewList() *List {
	return &List{list: make([]*Host, 0), mu: new(sync.RWMutex)}
}

func (l *List) Add(h *Host) {
	l.mu.Lock()
	l.list = append(l.list, h)
	l.mu.Unlock()
}

func (l *List) SetError(err error) {
	l.mu.Lock()
	l.err = err
	l.mu.Unlock()
}

func (l *List) GetError() error {
	l.mu.RLock()
	err := l.err
	l.mu.RUnlock()
	return err
}

func (l *List) Stat() int64 {
	var i int64 = 0
	for _, h := range l.list {
		h.mu.RLock()
		i += h.Sent
		h.mu.RUnlock()
	}
	return i
}

func (l *List) List() []*Host {
	var list []*Host
	l.mu.RLock()
	for _, h := range l.list {
		if h.Recv > 0 || h.Hostname != "" || h.PingError != nil {
			list = append(list, h)
		}
	}
	l.mu.RUnlock()

	sort.SliceStable(list, func(i, j int) bool {
		iNum, _ := binary.Uvarint(list[i].IP)
		jNum, _ := binary.Uvarint(list[j].IP)
		return iNum < jNum
	})

	return list
}
