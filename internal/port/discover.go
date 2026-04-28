//go:build windows

package port

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type PortInfo struct {
	LocalPort   int    `json:"local_port"`
	ProcessName string `json:"process_name"`
	ProcessPID  int    `json:"process_pid"`
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}
	return cmd
}

func DiscoverGamePorts() ([]PortInfo, error) {
	cmd := hiddenCmd("netstat", "-ano")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("netstat: %w", err)
	}

	pidNameMap := buildPidNameMap()

	lines := strings.Split(string(out), "\n")
	var results []PortInfo
	seen := map[int]bool{}

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		proto := fields[0]
		isUDP := strings.HasPrefix(proto, "UDP")
		if isUDP {
			if fields[2] != "*:*" {
				continue
			}
		} else {
			if !strings.Contains(line, "LISTENING") || len(fields) < 5 {
				continue
			}
		}
		addr := fields[1]
		pidStr := fields[len(fields)-1]

		_, portStr, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}

		var port int
		if n, _ := fmt.Sscanf(portStr, "%d", &port); n != 1 {
			continue
		}
		if port < 1000 || port > 65535 || seen[port] {
			continue
		}

		procName := pidNameMap[pidStr]
		if procName == "" {
			continue
		}

		seen[port] = true
		pidInt, _ := strconv.Atoi(pidStr)
		results = append(results, PortInfo{
			LocalPort:   port,
			ProcessName: procName,
			ProcessPID:  pidInt,
		})
	}
	return results, nil
}

func buildPidNameMap() map[string]string {
	cmd := hiddenCmd("tasklist", "/NH", "/FO", "CSV")
	out, err := cmd.Output()
	if err != nil {
		return map[string]string{}
	}

	m := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, `","`)
		if len(parts) < 2 {
			continue
		}
		name := strings.Trim(parts[0], `"`)
		pid := strings.Trim(parts[1], `"`)
		if name != "" && pid != "" {
			m[pid] = name
		}
	}
	return m
}
