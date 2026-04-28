package orchestrator

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"

	"springlink/internal/config"
	"springlink/internal/port"

	"github.com/pelletier/go-toml/v2"
)

type CmdConfig struct {
	Name    string
	Program string
	Args    []string
	Dir     string
	Env     []string
	Cleanup func()
}

type HealthCheck func() bool

type Step struct {
	Name           string
	Cmd            CmdConfig
	Health         HealthCheck
	Cleanup        func()
	AllocatedPorts []int
}

type ClientResult struct {
	Steps     []Step
	ConnAddr  string
	LocalAddr string
}

func execDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func findBinary(name string, binaryPath string) string {
	dir := execDir()
	var candidates []string
	if binaryPath != "" {
		candidates = append(candidates,
			filepath.Join(binaryPath, name),
			filepath.Join(binaryPath, name+".exe"),
		)
	}
	candidates = append(candidates,
		filepath.Join(dir, "bin", name, name),
		filepath.Join(dir, "bin", name, name+".exe"),
		filepath.Join(dir, "bin", name),
		filepath.Join(dir, "bin", name+".exe"),
		filepath.Join(dir, name),
		filepath.Join(dir, name+".exe"),
	)
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	slog.Warn("binary not found in local paths, relying on system PATH", "binary", name)
	return name
}

func findAvailablePort(wanted int, protocol string) int {
	p, err := port.AllocateNear(wanted, protocol)
	if err != nil {
		return wanted
	}
	return p
}

func tcpHealthCheck(addr string, timeout time.Duration, retries int, interval time.Duration) HealthCheck {
	return func() bool {
		for i := 0; i < retries; i++ {
			conn, err := net.DialTimeout("tcp", addr, timeout)
			if err == nil {
				conn.Close()
				return true
			}
			time.Sleep(interval)
		}
		return false
	}
}

func udpHealthCheck(addr string, timeout time.Duration, retries int, interval time.Duration) HealthCheck {
	return func() bool {
		for i := 0; i < retries; i++ {
			conn, err := net.DialTimeout("udp", addr, timeout)
			if err != nil {
				time.Sleep(interval)
				continue
			}
			conn.SetReadDeadline(time.Now().Add(timeout))
			conn.Write([]byte{0})
			buf := make([]byte, 1)
			_, err = conn.Read(buf)
			conn.Close()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					return true
				}
				time.Sleep(interval)
				continue
			}
			return true
		}
		return false
	}
}

func buildWstunnelServer(listenPort int, srv config.ServiceConfig, binaryPath string) CmdConfig {
	if listenPort == 0 {
		listenPort = 443
	}
	args := []string{
		"server",
		fmt.Sprintf("ws://0.0.0.0:%d", listenPort),
	}
	return CmdConfig{
		Name:    fmt.Sprintf("wstunnel-srv-%d", srv.LocalPort),
		Program: findBinary("wstunnel", binaryPath),
		Args:    args,
	}
}

func buildWstunnelClient(localPort int, proto string, gamePort int, serverAddr string, serverPort int, binaryPath string) CmdConfig {
	return CmdConfig{
		Name:    fmt.Sprintf("wstunnel-cli-%d", localPort),
		Program: findBinary("wstunnel", binaryPath),
		Args: []string{
			"client",
			fmt.Sprintf("--local-to-remote=%s://127.0.0.1:%d:127.0.0.1:%d", proto, localPort, gamePort),
			fmt.Sprintf("ws://%s:%d", serverAddr, serverPort),
		},
	}
}

func buildWstunnelRemoteClient(localPort int, proto string, remotePort int, serverAddr string, wsPort int, binaryPath string) CmdConfig {
	return CmdConfig{
		Name:    fmt.Sprintf("wstunnel-remote-%d->%d", localPort, remotePort),
		Program: findBinary("wstunnel", binaryPath),
		Args: []string{
			"client",
			fmt.Sprintf("--remote-to-local=%s://0.0.0.0:%d:127.0.0.1:%d", proto, remotePort, localPort),
			fmt.Sprintf("ws://%s:%d", serverAddr, wsPort),
		},
	}
}

type frpcConfig struct {
	ServerAddr string      `toml:"serverAddr"`
	ServerPort int         `toml:"serverPort"`
	Auth       *frpcAuth   `toml:"auth,omitempty"`
	Proxies    []frpcProxy `toml:"proxies"`
}

type frpcAuth struct {
	Method            string   `toml:"method"`
	Token             string   `toml:"token"`
	AdditionalScopes  []string `toml:"additionalScopes,omitempty"`
}

type frpcProxy struct {
	Name       string `toml:"name"`
	Type       string `toml:"type"`
	LocalIP    string `toml:"localIP"`
	LocalPort  int    `toml:"localPort"`
	RemotePort int    `toml:"remotePort"`
}

func buildFrpcProxy(localPort int, remotePort int, proto string, name string, srv config.ServiceConfig, binaryPath string) (*CmdConfig, error) {
	proxyCfg := frpcConfig{
		ServerAddr: srv.ServAddr,
		ServerPort: srv.FrpsPort,
		Proxies: []frpcProxy{
			{
				Name:       name,
				Type:       proto,
				LocalIP:    "127.0.0.1",
				LocalPort:  localPort,
				RemotePort: remotePort,
			},
		},
	}
	if srv.FrpsToken != "" {
		proxyCfg.Auth = &frpcAuth{
			Method:           "token",
			Token:            srv.FrpsToken,
			AdditionalScopes: []string{"HeartBeats", "NewWorkConns"},
		}
	}
	return writeFrpcConfig("proxy-"+name, &proxyCfg, binaryPath)
}

func writeFrpcConfig(kind string, v interface{}, binaryPath string) (*CmdConfig, error) {
	f, err := os.CreateTemp("", fmt.Sprintf("springlink-frpc-%s-*.toml", kind))
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	path := f.Name()

	data, err := toml.Marshal(v)
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("encode frpc config: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return nil, fmt.Errorf("write frpc config: %w", err)
	}
	f.Close()

	return &CmdConfig{
		Name:    fmt.Sprintf("frpc-%s", kind),
		Program: findBinary("frpc", binaryPath),
		Args:    []string{"-c", path},
		Cleanup: func() { os.Remove(path) },
	}, nil
}

func BuildCommands(srv config.ServiceConfig, cfg config.Config) ([]Step, error) {
	var steps []Step
	var wstunnelPort int

	if srv.Transport == "wstunnel" {
		wsPort := srv.WstunnelPort
		if wsPort == 0 {
			wsPort = 443
		}
		remotePort := srv.RemotePort
		if remotePort == 0 {
			remotePort = srv.LocalPort
		}
		wscmd := buildWstunnelRemoteClient(srv.LocalPort, srv.Protocol, remotePort, srv.ServAddr, wsPort, cfg.General.BinaryPath)

		tunAddr := fmt.Sprintf("%s:%d", srv.ServAddr, remotePort)
		var health HealthCheck
		if srv.Protocol == "tcp" {
			health = tcpHealthCheck(tunAddr, 2*time.Second, 10, 500*time.Millisecond)
		} else {
			health = udpHealthCheck(tunAddr, 2*time.Second, 10, 500*time.Millisecond)
		}

		steps = append(steps, Step{
			Name:   wscmd.Name,
			Cmd:    wscmd,
			Health: health,
		})
		return steps, nil
	}

	if srv.ConnectMethod == "wstunnel" {
		listenPort := srv.WstunnelPort
		var allocatedPorts []int
		if listenPort == 0 {
			p, err := port.Allocate(srv.Protocol)
			if err != nil {
				return nil, fmt.Errorf("allocate wstunnel port: %w", err)
			}
			listenPort = p
			allocatedPorts = append(allocatedPorts, p)
		}
		wstunnelPort = listenPort
		wscmd := buildWstunnelServer(listenPort, srv, cfg.General.BinaryPath)
		steps = append(steps, Step{
			Name:           wscmd.Name,
			Cmd:            wscmd,
			AllocatedPorts: allocatedPorts,
			Health: tcpHealthCheck(
				fmt.Sprintf("127.0.0.1:%d", wstunnelPort),
				2*time.Second, 10, 500*time.Millisecond,
			),
		})
	}

	if srv.Transport == "frp" {
		localPort := srv.LocalPort
		if wstunnelPort != 0 {
			localPort = wstunnelPort
		}
		proxyProto := "tcp"
		if wstunnelPort == 0 {
			proxyProto = srv.Protocol
		}
		remotePort := srv.RemotePort
		if remotePort == 0 {
			remotePort = localPort
		}
		proxyName := fmt.Sprintf("%s-%d", srv.Name, srv.LocalPort)
		frpcCmd, err := buildFrpcProxy(localPort, remotePort, proxyProto, proxyName, srv, cfg.General.BinaryPath)
		if err != nil {
			return nil, err
		}
		steps = append(steps, Step{
			Name:    frpcCmd.Name,
			Cmd:     *frpcCmd,
			Cleanup: frpcCmd.Cleanup,
		})
	}

	return steps, nil
}

func BuildClientCommands(srv config.ClientServiceConfig, cfg config.Config, serverAddr string) (*ClientResult, error) {
	binaryPath := cfg.General.BinaryPath
	switch {
	case srv.ConnectMethod == "wstunnel":
		targetPort := srv.WstunnelPort
		if targetPort == 0 {
			targetPort = 443
		}
		gamePort := srv.RemotePort
		if gamePort == 0 {
			gamePort = srv.LocalPort
		}
		localPort := findAvailablePort(gamePort, srv.Protocol)
		localAddr := fmt.Sprintf("127.0.0.1:%d", localPort)
		var health HealthCheck
		if srv.Protocol == "tcp" {
			health = tcpHealthCheck(localAddr, 2*time.Second, 10, 500*time.Millisecond)
		} else {
			health = udpHealthCheck(localAddr, 2*time.Second, 10, 500*time.Millisecond)
		}
		steps := []Step{{
			Name:           fmt.Sprintf("wstunnel-cli-%d", localPort),
			Cmd:            buildWstunnelClient(localPort, srv.Protocol, gamePort, serverAddr, targetPort, binaryPath),
			Health:         health,
			AllocatedPorts: []int{localPort},
		}}
		return &ClientResult{Steps: steps, LocalAddr: localAddr}, nil

	default:
		return &ClientResult{
			ConnAddr: fmt.Sprintf("%s:%d", serverAddr, srv.RemotePort),
		}, nil
	}
}
