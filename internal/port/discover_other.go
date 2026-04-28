//go:build !windows

package port

import "fmt"

func DiscoverGamePorts() ([]PortInfo, error) {
	return nil, fmt.Errorf("game port discovery is not supported on this platform")
}
