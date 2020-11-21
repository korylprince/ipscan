package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"regexp"
	"runtime"
	"time"

	"github.com/korylprince/ipnetgen"
	"github.com/korylprince/ipscan/ping"
	"github.com/korylprince/ipscan/resolve"
)

var ValidHostnameRegex = regexp.MustCompile(`^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9])\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\-]*[A-Za-z0-9])$`)

func main() {
	pingers := flag.Int("pingers", runtime.NumCPU()*2, "Number of concurrent pingers to run")
	resolvers := flag.Int("resolvers", runtime.NumCPU()*4, "Number of concurrent resolvers to run")
	buffer := flag.Int("buffer", 1024, "Size of service buffers")
	timeout := flag.Int("timeout", 1000, "ICMP timeout (in ms)")
	interval := flag.Int("interval", 10, "Number of seconds between pings")

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [OPTION]... DEST...\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "DEST can be a hostname, IP address, or CIDR")
	}

	flag.Parse()

	var cidrs []*ipnetgen.IPNetGenerator
	var ips []net.IP
	var hosts []string

	for _, a := range flag.Args() {
		if cidr, err := ipnetgen.New(a); err == nil {
			cidrs = append(cidrs, cidr)
			continue
		}
		if ip := net.ParseIP(a); ip != nil {
			ips = append(ips, ip)
			continue
		}
		if ValidHostnameRegex.MatchString(a) {
			hosts = append(hosts, a)
			continue
		}
		flag.Usage()
		fmt.Println("Unable to parse destination:", a)
		os.Exit(1)
	}

	if len(flag.Args()) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	list := NewList()

	resolver := resolve.NewService(*resolvers, *buffer)
	pinger, listeners, err := ping.NewService(*pingers, *buffer, time.Millisecond*time.Duration(*timeout), list.SetError)
	if err != nil {
		fmt.Println("Unable to start PingService:", err)
		os.Exit(1)
	}

	for _, cidr := range cidrs {
		for ip := cidr.Next(); ip != nil; ip = cidr.Next() {
			h := NewHost(ip, "")
			go h.LookupAddr(resolver)
			go h.StartPing(pinger, time.Second*time.Duration(*interval))
			list.Add(h)
		}
	}

	for _, ip := range ips {
		h := NewHost(ip, "")
		go h.LookupAddr(resolver)
		go h.StartPing(pinger, time.Second*time.Duration(*interval))
		list.Add(h)
	}

	for _, host := range hosts {
		h := NewHost(nil, host)
		list.Add(h)
		go func(hostname string) {
			ips, err := resolver.LookupIP(hostname)
			if err != nil {
				h.mu.Lock()
				h.ResolveError = err
				h.mu.Unlock()
				return
			}
			if len(ips) > 0 {
				h.mu.Lock()
				h.IP = ips[0]
				h.mu.Unlock()
				go h.StartPing(pinger, time.Second*time.Duration(*interval))
			}
			for _, ip := range ips[1:] {
				h := NewHost(ip, hostname)
				go h.StartPing(pinger, time.Second*time.Duration(*interval))
				list.Add(h)
			}
		}(host)
	}

	ui(listeners, list)
}
