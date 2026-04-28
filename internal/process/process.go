package process

import (
	"context"
	"os/exec"

	"springlink/internal/config"
	"springlink/internal/orchestrator"
)

type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

type ClientOptions struct {
	ServAddr string
}

type LogCallback func(serviceID int, cmdName string, line string)

type StateChangeCallback func()

type ServiceState struct {
	Index  int    `json:"index"`
	Name   string `json:"name"`
	Status Status `json:"status"`
}

type ServiceRuntime struct {
	Cfg    config.ServiceConfig
	Cmds   []*exec.Cmd
	Steps  []orchestrator.Step
	Status Status
	Cancel context.CancelFunc
}
