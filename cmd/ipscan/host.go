package main

import (
	"net"
	"sync"
	"time"

	"github.com/korylprince/ipscan/ping"
	"github.com/korylprince/ipscan/resolve"
)

type Host struct {
	IP           net.IP
	Hostname     string
	ResolveError error
	PingError    error

	Sent int64
	Recv int64
	Last *ping.Ping
	Min  time.Duration
	Max  time.Duration
	Avg  time.Duration

	mu        *sync.RWMutex
	resolving bool
}

func NewHost(ip net.IP, host string) *Host {
	return &Host{
		Hostname: host,
		IP:       ip,
		mu:       new(sync.RWMutex),
	}
}

func (h *Host) LookupAddr(resolver *resolve.Service) {
	h.mu.Lock()
	h.resolving = true
	ip := h.IP
	h.mu.Unlock()

	hostname, err := resolver.LookupAddr(ip)

	h.mu.Lock()
	defer h.mu.Unlock()

	if err == nil {
		h.Hostname = hostname
	}

	h.resolving = false
}

func (h *Host) StartPing(pinger *ping.Service, interval time.Duration) {
	for {
		h.mu.RLock()
		ip := h.IP
		h.mu.RUnlock()

		p, err := pinger.Ping(ip)
		h.mu.Lock()

		h.PingError = err
		h.Sent++
		h.Last = p
		if p.RecvTime != nil {
			h.Recv++
			ttl := p.RecvTime.Sub(p.SentTime)
			if h.Min > ttl || h.Min == 0 {
				h.Min = ttl
			}
			if h.Max < ttl {
				h.Max = ttl
			}

			h.Avg += ((ttl - h.Avg) / time.Duration(h.Recv))
		}

		h.mu.Unlock()

		time.Sleep(interval)
	}
}
