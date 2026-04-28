package config

import "github.com/pelletier/go-toml/v2"

func migrate(cfg *Config) {
	if cfg.Version == 0 {
		cfg.Version = 1
	}
}

// migrateEncapsulation copies old "encapsulation" TOML values to "connect_method"
// after the field rename v1→v2. Called from LoadConfig with raw TOML bytes.
func migrateEncapsulation(cfg *Config, raw []byte) {
	type oldSvc struct {
		Encap *string `toml:"encapsulation"`
	}
	type oldCfg struct {
		Srvs   []oldSvc `toml:"services"`
		Client *struct {
			Srvs []oldSvc `toml:"services"`
		} `toml:"client"`
	}
	var old oldCfg
	if err := toml.Unmarshal(raw, &old); err != nil {
		return
	}
	for i, s := range old.Srvs {
		if s.Encap != nil && i < len(cfg.Services) && cfg.Services[i].ConnectMethod == "" {
			cfg.Services[i].ConnectMethod = *s.Encap
		}
	}
	if old.Client != nil {
		for i, s := range old.Client.Srvs {
			if s.Encap != nil && i < len(cfg.Client.Services) && cfg.Client.Services[i].ConnectMethod == "" {
				cfg.Client.Services[i].ConnectMethod = *s.Encap
			}
		}
	}
}

// migrateServAddr unifies frps_addr + server_addr → serv_addr (v2→v3).
func migrateServAddr(cfg *Config, raw []byte) {
	type oldSvc struct {
		FrpsAddr *string `toml:"frps_addr"`
		SrvAddr  *string `toml:"server_addr"`
	}
	type oldClCfg struct {
		SrvAddr *string   `toml:"server_addr"`
		Srvs    []oldSvc `toml:"services"`
	}
	type oldCfg struct {
		Srvs   []oldSvc `toml:"services"`
		Client *oldClCfg  `toml:"client"`
	}
	var old oldCfg
	if err := toml.Unmarshal(raw, &old); err != nil {
		return
	}
	for i, s := range old.Srvs {
		if i >= len(cfg.Services) {
			break
		}
		if cfg.Services[i].ServAddr != "" {
			continue
		}
		if s.FrpsAddr != nil {
			cfg.Services[i].ServAddr = *s.FrpsAddr
		} else if s.SrvAddr != nil {
			cfg.Services[i].ServAddr = *s.SrvAddr
		}
	}
	if old.Client != nil {
		if cfg.Client.ServAddr == "" && old.Client.SrvAddr != nil {
			cfg.Client.ServAddr = *old.Client.SrvAddr
		}
		for i, s := range old.Client.Srvs {
			if i >= len(cfg.Client.Services) {
				break
			}
			if cfg.Client.Services[i].ServAddr != "" {
				continue
			}
			if s.FrpsAddr != nil {
				cfg.Client.Services[i].ServAddr = *s.FrpsAddr
			} else if s.SrvAddr != nil {
				cfg.Client.Services[i].ServAddr = *s.SrvAddr
			}
		}
	}
}
