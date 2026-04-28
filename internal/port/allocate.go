package port

import (
	"fmt"
	"net"
	"sync"
)

var (
	mu         sync.Mutex
	allocated  = map[int]bool{}
	rangeStart = 10000
	rangeEnd   = 20000
)

func portFree(port int, protocol string) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	ln.Close()
	if protocol == "udp" && !portFreeUDP(port) {
		return false
	}
	return true
}

func AllocateNear(wanted int, protocol string) (int, error) {
	mu.Lock()
	if !allocated[wanted] && portFree(wanted, protocol) {
		allocated[wanted] = true
		mu.Unlock()
		return wanted, nil
	}
	mu.Unlock()
	maxProbe := wanted + 100
	if maxProbe > rangeEnd {
		maxProbe = rangeEnd
	}
	for p := wanted + 1; p <= maxProbe; p++ {
		if portFree(p, protocol) {
			mu.Lock()
			if !allocated[p] {
				allocated[p] = true
				mu.Unlock()
				return p, nil
			}
			mu.Unlock()
		}
	}
	return Allocate(protocol)
}

func Allocate(protocol string) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	for p := rangeStart; p <= rangeEnd; p++ {
		if allocated[p] {
			continue
		}
		if !portFree(p, protocol) {
			continue
		}
		allocated[p] = true
		return p, nil
	}
	return 0, fmt.Errorf("no free port in %d-%d", rangeStart, rangeEnd)
}

func Free(port int) {
	mu.Lock()
	defer mu.Unlock()
	delete(allocated, port)
}

func Reserve(port int, protocol string) bool {
	mu.Lock()
	defer mu.Unlock()
	if allocated[port] {
		return false
	}
	if !portFree(port, protocol) {
		return false
	}
	allocated[port] = true
	return true
}
