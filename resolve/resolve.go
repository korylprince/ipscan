package resolve

import (
	"errors"
	"net"
)

type host struct {
	Hostname string
	IPs      []net.IP
	Error    error
	callback chan *host
}

//Service is a type-safe service to resolve hosts concurrently
type Service struct {
	in chan *host

	poolIn  chan chan *host
	poolOut chan chan *host
}

func (s *Service) pooler() {
	for {
		s.poolOut <- <-s.poolIn
	}
}

func (s *Service) resolver() {
	for host := range s.in {
		//resolve hostname
		if host.IPs == nil {
			ips, err := net.LookupIP(host.Hostname)
			if err != nil {
				host.Error = err
				host.callback <- host
				continue
			}

			host.IPs = make([]net.IP, 0)
			for _, ip := range ips {
				if ipv4 := ip.To4(); ipv4 != nil {
					host.IPs = append(host.IPs, ipv4)
				}
			}
			host.callback <- host

			//reverse lookup
		} else {
			hosts, err := net.LookupAddr(host.IPs[0].String())
			if err != nil {
				host.Error = err
				host.callback <- host
				continue
			}
			if len(hosts) == 0 {
				host.Error = errors.New("No hosts found")
				host.callback <- host
				continue
			}
			host.Hostname = hosts[0]
			host.callback <- host
		}
	}
}

//NewService returns a new *Service with the given amount of workers and buffer size
func NewService(workers, buffer int) *Service {
	s := &Service{
		in:      make(chan *host),
		poolIn:  make(chan chan *host, buffer),
		poolOut: make(chan chan *host, buffer),
	}

	for i := 0; i < buffer; i++ {
		s.poolIn <- make(chan *host)
	}
	go s.pooler()

	for i := 0; i < workers; i++ {
		go s.resolver()
	}

	return s
}

//LookupIP returns the net.IPs resolved from given hostname, or an error if one occurred
func (s *Service) LookupIP(hostname string) ([]net.IP, error) {
	callback := <-s.poolOut
	s.in <- &host{Hostname: hostname, callback: callback}
	host := <-callback
	s.poolIn <- callback
	return host.IPs, host.Error
}

//LookupAddr returns the reverse lookup hostname of the given IP or an error if one occurred
func (s *Service) LookupAddr(addr net.IP) (string, error) {
	callback := <-s.poolOut
	s.in <- &host{IPs: []net.IP{addr}, callback: callback}
	host := <-callback
	s.poolIn <- callback
	return host.Hostname, host.Error
}
