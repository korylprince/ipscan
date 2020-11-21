package ping

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/korylprince/go-icmpv4/v2/echo"
)

const ICMPEchoRequestIdentifier uint16 = 0x3039

//Ping represents an ICMP echo request
type Ping struct {
	IP       net.IP
	Sequence uint16
	SentTime time.Time
	//RecvTime will be non-nil if a echo response was received
	RecvTime *time.Time
	err      error
	callback chan *Ping
}

//Service is a type-safe service to send pings concurrently
type Service struct {
	sequence chan uint16

	poolIn  chan chan *Ping
	poolOut chan chan *Ping

	requests chan *Ping

	packets chan *echo.IPPacket

	pending   map[uint16]*Ping
	pendingMu *sync.Mutex

	errors     chan error
	errHandler func(error)
}

func (s *Service) sequencer() {
	var seq uint16 = 0
	for {
		s.sequence <- seq
		seq++
	}
}

func (s *Service) nextSequence() uint16 {
	return <-(s.sequence)
}

func (s *Service) pooler() {
	for {
		s.poolOut <- <-s.poolIn
	}
}

func (s *Service) requester() {
	for req := range s.requests {
		seq := s.nextSequence()
		t := time.Now()
		req.Sequence = seq
		req.SentTime = t
		ip := req.IP
		s.pendingMu.Lock()
		s.pending[seq] = req
		s.pendingMu.Unlock()
		err := echo.Send(nil, &net.IPAddr{IP: ip}, ICMPEchoRequestIdentifier, seq)
		if err != nil {
			s.pendingMu.Lock()
			s.pending[seq].err = fmt.Errorf("Unable to send echo request: %w", err)
			s.pendingMu.Unlock()
		}
	}
}

func (s *Service) receiver() {
	for pk := range s.packets {
		recv := time.Now()
		if pk.Identifier() != ICMPEchoRequestIdentifier {
			continue
		}
		s.pendingMu.Lock()
		if req, ok := s.pending[pk.Sequence()]; ok {
			if req.IP.Equal(pk.RemoteAddr.IP) {
				req.RecvTime = &recv
				req.callback <- req
				delete(s.pending, pk.Sequence())
			}
		}
		s.pendingMu.Unlock()
	}
}

func (s *Service) scavenger(timeout time.Duration) {
	for {
		time.Sleep(timeout / 2)
		s.pendingMu.Lock()

		for seq, req := range s.pending {
			if time.Now().After(req.SentTime.Add(timeout)) {
				req.callback <- req
				delete(s.pending, seq)
			}
		}

		s.pendingMu.Unlock()
	}
}

func (s *Service) errorHandler() {
	for {
		if s.errHandler == nil {
			<-s.errors
		} else {
			s.errHandler(<-s.errors)
		}
	}
}

//NewService returns a new *Service with the given amount of workers, buffer size, ping timeout and an error handler.
//If errHandler is nil, service errors will be silently dropped
func NewService(workers, buffer int, timeout time.Duration, errHandler func(error)) (*Service, []*net.IPAddr, error) {
	s := &Service{
		sequence:   make(chan uint16),
		poolIn:     make(chan chan *Ping, buffer),
		poolOut:    make(chan chan *Ping, buffer),
		requests:   make(chan *Ping, buffer),
		packets:    make(chan *echo.IPPacket, buffer),
		pending:    make(map[uint16]*Ping),
		pendingMu:  new(sync.Mutex),
		errors:     make(chan error),
		errHandler: errHandler,
	}

	ips, err := echo.ListenerAll(s.packets, s.errors, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Unable to start listeners: %w", err)
	}

	go s.sequencer()

	for i := 0; i < buffer; i++ {
		s.poolIn <- make(chan *Ping)
	}
	go s.pooler()

	for i := 0; i < workers; i++ {
		go s.requester()
	}

	go s.receiver()
	go s.scavenger(timeout)
	go s.errorHandler()

	return s, ips, nil
}

//Ping sends one ICMP echo request to ip and returns a *Ping, or an error if one occurred
func (s *Service) Ping(ip net.IP) (*Ping, error) {
	callback := <-s.poolOut
	s.requests <- &Ping{IP: ip, callback: callback}
	ping := <-callback
	s.poolIn <- callback
	return ping, ping.err
}
