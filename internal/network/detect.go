package network

import (
	"fmt"
	"net"
	"time"

	"github.com/pion/stun"
)

type DetectResult struct {
	PublicIP    string `json:"public_ip"`
	HasPublicIP bool   `json:"has_public_ip"`
}

func DetectPublicIP(serverAddr string) (*DetectResult, error) {
	if serverAddr == "" {
		serverAddr = "stun.l.google.com:19302"
	}
	c, err := net.DialTimeout("udp4", serverAddr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("stun dial: %w", err)
	}
	conn, err := stun.NewClient(c)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("stun client: %w", err)
	}
	defer conn.Close()

	ipCh := make(chan string, 1)

	message := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
	if err := conn.Do(message, func(res stun.Event) {
		if res.Error != nil {
			ipCh <- ""
			return
		}
		var xorAddr stun.XORMappedAddress
		if err := xorAddr.GetFrom(res.Message); err != nil {
			ipCh <- ""
			return
		}
		ipCh <- xorAddr.IP.String()
	}); err != nil {
		return nil, fmt.Errorf("stun request: %w", err)
	}

	publicIP := <-ipCh
	result := &DetectResult{PublicIP: publicIP}

	if result.PublicIP == "" {
		return result, nil
	}

	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		if ipNet.IP.Equal(net.ParseIP(result.PublicIP)) {
			result.HasPublicIP = true
			break
		}
	}

	return result, nil
}
