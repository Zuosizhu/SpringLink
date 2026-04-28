//go:build !windows

package port

import (
	"fmt"
	"net"
)

func portFreeUDP(port int) bool {
	pc, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	pc.Close()
	return true
}
