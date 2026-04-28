package config

import (
	"os"

	"github.com/pelletier/go-toml/v2"
)

type General struct {
	HasPublicIP  bool   `toml:"has_public_ip" json:"has_public_ip"`
	PublicIP     string `toml:"public_ip" json:"public_ip"`
	AutoDetectIP bool   `toml:"auto_detect_ip" json:"auto_detect_ip"`
	BinaryPath   string `toml:"binary_path" json:"binary_path"`
	ActiveTab    string `toml:"active_tab" json:"active_tab"`
	Guided       bool   `toml:"guided" json:"guided"`
}

type ServiceConfig struct {
	Name              string `toml:"name" json:"name"`
	Protocol          string `toml:"protocol" json:"protocol"`
	Enabled           bool   `toml:"enabled" json:"enabled"`
	LocalPort         int    `toml:"local_port" json:"local_port"`
	Transport         string `toml:"transport" json:"transport"`
	ServAddr          string `toml:"serv_addr" json:"serv_addr"`
	FrpsPort          int    `toml:"frps_port" json:"frps_port"`
	FrpsToken         string `toml:"frps_token" json:"frps_token"`
	RemotePort        int    `toml:"remote_port" json:"remote_port"`
	ConnectMethod     string `toml:"connect_method" json:"connect_method"`
	WstunnelLocalPort int    `toml:"wstunnel_local_port" json:"wstunnel_local_port"`
	WstunnelPort      int    `toml:"wstunnel_port" json:"wstunnel_port"`
}

type ClientServiceConfig struct {
	Name              string `toml:"name" json:"name"`
	Protocol          string `toml:"protocol" json:"protocol"`
	Enabled           bool   `toml:"enabled" json:"enabled"`
	LocalPort         int    `toml:"local_port" json:"local_port"`
	RemotePort        int    `toml:"remote_port" json:"remote_port"`
	ConnectMethod     string `toml:"connect_method" json:"connect_method"`
	WstunnelLocalPort int    `toml:"wstunnel_local_port" json:"wstunnel_local_port"`
	WstunnelPort      int    `toml:"wstunnel_port" json:"wstunnel_port"`
	ServAddr          string `toml:"serv_addr" json:"serv_addr"`
}

func (c ClientServiceConfig) ToServiceConfig() ServiceConfig {
	return ServiceConfig{
		Name:              c.Name,
		Protocol:          c.Protocol,
		Enabled:           c.Enabled,
		LocalPort:         c.LocalPort,
		RemotePort:        c.RemotePort,
		ConnectMethod:     c.ConnectMethod,
		WstunnelLocalPort: c.WstunnelLocalPort,
		WstunnelPort:      c.WstunnelPort,
		ServAddr:          c.ServAddr,
	}
}

type ClientConfig struct {
	ServAddr  string                `toml:"serv_addr" json:"serv_addr"`
	Services   []ClientServiceConfig `toml:"services" json:"services"`
}

type Config struct {
	Version  int             `toml:"version" json:"version"`
	General  General         `toml:"general" json:"general"`
	Services []ServiceConfig `toml:"services" json:"services"`
	Client   ClientConfig    `toml:"client" json:"client"`
}

func DefaultConfig() *Config {
	return &Config{
		Version: 3,
		General: General{
			HasPublicIP:  false,
			AutoDetectIP: true,
		},
		Services: []ServiceConfig{},
		Client:   ClientConfig{},
	}
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	migrate(&cfg)
	if cfg.Version == 1 {
		migrateEncapsulation(&cfg, data)
		cfg.Version = 2
	}
	if cfg.Version == 2 {
		migrateServAddr(&cfg, data)
		cfg.Version = 3
	}
	return &cfg, nil
}

func SaveConfig(path string, cfg *Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
