package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"springlink/internal/codec"
	"springlink/internal/config"
	"springlink/internal/network"
	"springlink/internal/port"
	"springlink/internal/process"
	"springlink/internal/tray"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func init() {
	cleanupOrphanedConfigs()
}

func cleanupOrphanedConfigs() {
	matches, err := filepath.Glob(filepath.Join(os.TempDir(), "springlink-frpc-*.toml"))
	if err != nil {
		slog.Error("failed to glob orphaned configs", "err", err)
		return
	}
	for _, m := range matches {
		os.Remove(m)
	}
}

type App struct {
	ctx        context.Context
	mu         sync.RWMutex
	config     *config.Config
	configPath string
	serverPM   *process.ServerManager
	clientPM   *process.ClientManager
	closing    int32
}

type ClientStartResult struct {
	ProcId    int    `json:"procId"`
	ConnAddr  string `json:"connAddr"`
	LocalAddr string `json:"localAddr"`
}

func NewApp() *App {
	return &App{
		config: config.DefaultConfig(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	go tray.Start(
		func() { runtime.Show(a.ctx) },
		func() {
			runtime.EventsEmit(a.ctx, "tray-quit-requested", nil)
		},
	)

	go func() {
		time.Sleep(500 * time.Millisecond)
		tray.SetWindowIcon()
	}()



	exePath, err := os.Executable()
	if err != nil {
		slog.Error("failed to get executable path", "error", err)
		return
	}
	baseDir := filepath.Dir(exePath)
	a.configPath = filepath.Join(baseDir, "config.toml")

	loaded, err := config.LoadConfig(a.configPath)
	if err != nil {
		slog.Warn("failed to load config, using defaults", "path", a.configPath, "error", err)
	} else {
		a.config = loaded
	}

	a.serverPM = process.NewServerManager(
		func(serviceID int, cmdName string, line string) {
			runtime.EventsEmit(a.ctx, "service-log", map[string]interface{}{
				"serviceId": serviceID,
				"cmd":       cmdName,
				"line":      line,
				"type":      "server",
			})
		},
		func() {
			runtime.EventsEmit(a.ctx, "service-state-changed", nil)
		},
	)
	a.clientPM = process.NewClientManager(
		func(serviceID int, cmdName string, line string) {
			runtime.EventsEmit(a.ctx, "service-log", map[string]interface{}{
				"serviceId": serviceID,
				"cmd":       cmdName,
				"line":      line,
				"type":      "client",
			})
		},
		func() {
			runtime.EventsEmit(a.ctx, "service-state-changed", nil)
		},
	)
}

func (a *App) shutdown(ctx context.Context) {
	a.serverPM.StopAll()
	a.clientPM.StopAll()
}

func (a *App) MinimizeToTray() {
	runtime.Hide(a.ctx)
}

func (a *App) GetConfig() *config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

func (a *App) SaveConfig(cfg *config.Config) error {
	a.mu.Lock()
	a.config = cfg
	path := a.configPath
	a.mu.Unlock()
	return config.SaveConfig(path, cfg)
}

func (a *App) SelectConfigFile() string {
	file, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择服务端配置文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "TOML 配置", Pattern: "*.toml"},
		},
	})
	if err != nil {
		return ""
	}
	return file
}

type ServerStartResult struct {
	ProcId int `json:"procId"`
}

func (a *App) StartService(index int) (*ServerStartResult, error) {
	a.mu.RLock()
	if index < 0 || index >= len(a.config.Services) {
		a.mu.RUnlock()
		return nil, fmt.Errorf("invalid service index: %d", index)
	}
	srv := a.config.Services[index]
	cfg := *a.config
	a.mu.RUnlock()
	if srv.ConnectMethod == "raw" && srv.Transport == "direct" {
		return &ServerStartResult{ProcId: 0}, nil
	}
	procId, err := a.serverPM.StartService(index, srv, cfg)
	if err != nil {
		return nil, err
	}
	return &ServerStartResult{ProcId: procId}, nil
}

func (a *App) StopService(procId int) {
	a.serverPM.StopService(procId)
}

func (a *App) StartAll() []string {
	a.mu.RLock()
	services := make([]config.ServiceConfig, len(a.config.Services))
	copy(services, a.config.Services)
	cfg := *a.config
	a.mu.RUnlock()
	msgs := []string{}
	for i, srv := range services {
		if !srv.Enabled {
			continue
		}
		if srv.ConnectMethod == "raw" && srv.Transport == "direct" {
			continue
		}
		if _, err := a.serverPM.StartService(i, srv, cfg); err != nil {
			msgs = append(msgs, err.Error())
		}
	}
	return msgs
}

func (a *App) StopAllServers() {
	a.serverPM.StopAll()
}

func (a *App) GetServerStates() []process.ServiceState {
	return a.serverPM.GetStates()
}

func (a *App) GetClientStates() []process.ServiceState {
	return a.clientPM.GetStates()
}

func (a *App) StopClientService(index int) {
	a.clientPM.StopService(index)
}

func (a *App) StopAllClients() {
	a.clientPM.StopAll()
}

func (a *App) LoadConfigFile(path string) (*config.Config, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (a *App) ScanPorts() ([]port.PortInfo, error) {
	return port.DiscoverGamePorts()
}

func (a *App) DetectPublicIP(serverAddr string) (*network.DetectResult, error) {
	return network.DetectPublicIP(serverAddr)
}

func (a *App) StartClientService(svc config.ClientServiceConfig, serverAddr string) (*ClientStartResult, error) {
	procId, connAddr, localAddr, err := a.clientPM.StartClientService(svc, *a.config, process.ClientOptions{ServAddr: serverAddr})
	if err != nil {
		return nil, err
	}
	return &ClientStartResult{ProcId: procId, ConnAddr: connAddr, LocalAddr: localAddr}, nil
}

func (a *App) StartAllClients(services []config.ClientServiceConfig, serverAddr string) []string {
	return a.clientPM.StartAllClients(services, *a.config, process.ClientOptions{ServAddr: serverAddr})
}

func (a *App) ExportClientService(svc config.ClientServiceConfig) (string, error) {
	return codec.Encode(&codec.ConnectionCode{
		Version:      1,
		Name:         svc.Name,
		Protocol:     svc.Protocol,
		ConnectMethod: svc.ConnectMethod,
		LocalPort:    svc.LocalPort,
		RemoteHost:   svc.ServAddr,
		RemotePort:   svc.RemotePort,
		WstunnelPort: svc.WstunnelPort,
	})
}

func (a *App) ExportService(index int) (string, error) {
	a.mu.RLock()
	if index < 0 || index >= len(a.config.Services) {
		a.mu.RUnlock()
		return "", fmt.Errorf("invalid service index: %d", index)
	}
	srv := a.config.Services[index]
	a.mu.RUnlock()

	var remoteHost string
	var remotePort int
	wstunnelPort := 0

	switch {
	case srv.ConnectMethod == "raw" && srv.Transport == "direct":
		remoteHost = a.config.General.PublicIP
		remotePort = srv.LocalPort
	case srv.ConnectMethod == "raw" && srv.Transport == "frp":
		remoteHost = srv.ServAddr
		remotePort = srv.RemotePort
		if remotePort == 0 {
			remotePort = srv.LocalPort
		}
	case srv.ConnectMethod == "wstunnel" && srv.Transport == "direct":
		remoteHost = a.config.General.PublicIP
		remotePort = srv.LocalPort
		wstunnelPort = srv.WstunnelPort
		if wstunnelPort == 0 {
			wstunnelPort = 443
		}
	case srv.ConnectMethod == "wstunnel" && srv.Transport == "frp":
		remoteHost = srv.ServAddr
		remotePort = srv.LocalPort
		wstunnelPort = srv.RemotePort
		if wstunnelPort == 0 {
			wstunnelPort = 443
		}
	case srv.ConnectMethod == "raw" && srv.Transport == "wstunnel":
		remoteHost = srv.ServAddr
		remotePort = srv.RemotePort
		if remotePort == 0 {
			remotePort = srv.LocalPort
		}
	case srv.ConnectMethod == "wstunnel" && srv.Transport == "wstunnel":
		remoteHost = srv.ServAddr
		remotePort = srv.RemotePort
		if remotePort == 0 {
			remotePort = srv.LocalPort
		}
		wstunnelPort = srv.WstunnelPort
		if wstunnelPort == 0 {
			wstunnelPort = 443
		}
	default:
		return "", fmt.Errorf("unknown combination: %s + %s", srv.ConnectMethod, srv.Transport)
	}

	code := &codec.ConnectionCode{
		Version:      1,
		Name:         srv.Name,
		Protocol:     srv.Protocol,
		ConnectMethod: srv.ConnectMethod,
		Transport:    srv.Transport,
		LocalPort:    srv.LocalPort,
		RemoteHost:   remoteHost,
		RemotePort:   remotePort,
		WstunnelPort: wstunnelPort,
		ServAddr:     srv.ServAddr,
		FrpsPort:     srv.FrpsPort,
		FrpsToken:    srv.FrpsToken,
	}

	return codec.Encode(code)
}

func (a *App) PreviewService(codeStr string) (*codec.ConnectionCode, error) {
	return codec.Decode(codeStr)
}

func (a *App) ImportService(codeStr string) error {
	_, err := codec.Decode(codeStr)
	if err != nil {
		return fmt.Errorf("decode failed: %w", err)
	}
	return nil
}

func (a *App) CancelClose() {
	atomic.StoreInt32(&a.closing, 0)
}
