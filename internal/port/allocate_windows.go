//go:build windows

package port

import (
	"fmt"
	"net"
	"strings"
)

func portFreeUDP(port int) bool {
	pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	pc.Close()

	cmd := hiddenCmd("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return true
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if !strings.HasPrefix(fields[0], "UDP") {
			continue
		}
		_, portStr, err := net.SplitHostPort(fields[1])
		if err != nil {
			continue
		}
		var p int
		if n, _ := fmt.Sscanf(portStr, "%d", &p); n == 1 && p == port {
			return false
		}
	}
	return true
}
