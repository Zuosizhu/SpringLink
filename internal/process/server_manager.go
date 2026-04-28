package process

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"springlink/internal/config"
	"springlink/internal/orchestrator"
	"springlink/internal/port"
)

type ServerManager struct {
	mu            sync.Mutex
	services      map[int]*ServiceRuntime
	onLog         LogCallback
	onStateChange StateChangeCallback
	serverIndex   int
	freeIndices   []int
}

func NewServerManager(onLog LogCallback, onStateChange StateChangeCallback) *ServerManager {
	return &ServerManager{
		services:      make(map[int]*ServiceRuntime),
		onLog:         onLog,
		onStateChange: onStateChange,
	}
}

func (sm *ServerManager) StartService(index int, srv config.ServiceConfig, cfg config.Config) (int, error) {
	steps, err := orchestrator.BuildCommands(srv, cfg)
	if err != nil {
		return 0, fmt.Errorf("build commands: %w", err)
	}
	sm.mu.Lock()
	var procId int
	if n := len(sm.freeIndices); n > 0 {
		procId = sm.freeIndices[n-1]
		sm.freeIndices = sm.freeIndices[:n-1]
	} else {
		sm.serverIndex++
		procId = sm.serverIndex
	}
	sm.mu.Unlock()
	if err := sm.startCommands(procId, srv, steps); err != nil {
		return 0, err
	}
	return procId, nil
}

func (sm *ServerManager) StopService(procId int) {
	sm.mu.Lock()
	rt, ok := sm.services[procId]
	if !ok {
		sm.mu.Unlock()
		return
	}

	rt.Cancel()
	for _, cmd := range rt.Cmds {
		killProcessTree(cmd)
	}
	for i := len(rt.Steps) - 1; i >= 0; i-- {
		if rt.Steps[i].Cleanup != nil {
			rt.Steps[i].Cleanup()
		}
		for _, p := range rt.Steps[i].AllocatedPorts {
			port.Free(p)
		}
	}
	rt.Status = StatusStopped
	delete(sm.services, procId)
	sm.freeIndices = append(sm.freeIndices, procId)
	sm.mu.Unlock()
	if sm.onStateChange != nil {
		sm.onStateChange()
	}
}

func (sm *ServerManager) startCommands(procId int, srv config.ServiceConfig, steps []orchestrator.Step) error {
	sm.mu.Lock()
	if rt, ok := sm.services[procId]; ok && rt.Status == StatusRunning {
		sm.mu.Unlock()
		return fmt.Errorf("service %d is already running", procId)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &ServiceRuntime{
		Cfg:    srv,
		Steps:  steps,
		Status: StatusStarting,
		Cancel: cancel,
	}
	sm.services[procId] = rt
	sm.mu.Unlock()

	for _, step := range steps {
		cc := step.Cmd
		cmd := exec.CommandContext(ctx, cc.Program, cc.Args...)
		if cc.Dir != "" {
			cmd.Dir = cc.Dir
		}
		if cc.Env != nil {
			cmd.Env = append(os.Environ(), cc.Env...)
		}
		setProcessGroup(cmd)

		if sm.onLog != nil {
			sm.onLog(procId, cc.Name, fmt.Sprintf("启动命令: %s %s", cc.Program, strings.Join(cc.Args, " ")))
		}

		stdout, err := cmd.StdoutPipe()
		if err == nil {
			sm.startLogReader(procId, cc.Name, stdout)
		}
		stderr, err := cmd.StderrPipe()
		if err == nil {
			sm.startLogReader(procId, cc.Name, stderr)
		}

		if err := cmd.Start(); err != nil {
			sm.mu.Lock()
			for _, c := range rt.Cmds {
				killProcessTree(c)
			}
			rt.Status = StatusError
			sm.mu.Unlock()
			for i := len(steps) - 1; i >= 0; i-- {
				if steps[i].Cleanup != nil {
					steps[i].Cleanup()
				}
				for _, p := range steps[i].AllocatedPorts {
					port.Free(p)
				}
			}
			if sm.onStateChange != nil {
				sm.onStateChange()
			}
			cancel()
			return fmt.Errorf("start %s: %w", cc.Name, err)
		}

		sm.mu.Lock()
		rt.Cmds = append(rt.Cmds, cmd)
		sm.mu.Unlock()

		if step.Health != nil {
			if ok := step.Health(); !ok {
				sm.mu.Lock()
				for _, c := range rt.Cmds {
					killProcessTree(c)
				}
				rt.Status = StatusError
				sm.mu.Unlock()
				for i := len(steps) - 1; i >= 0; i-- {
					if steps[i].Cleanup != nil {
						steps[i].Cleanup()
					}
					for _, p := range steps[i].AllocatedPorts {
						port.Free(p)
					}
				}
				if sm.onStateChange != nil {
					sm.onStateChange()
				}
				cancel()
				return fmt.Errorf("health check failed for %s", step.Name)
			}
		}
	}

	sm.mu.Lock()
	rt.Status = StatusRunning
	sm.mu.Unlock()
	if sm.onStateChange != nil {
		sm.onStateChange()
	}

	go sm.waitForCompletion(procId, rt)
	return nil
}

func (sm *ServerManager) StartAll(services []config.ServiceConfig, cfg config.Config) []error {
	var errs []error
	for i, srv := range services {
		if !srv.Enabled {
			continue
		}
		if _, err := sm.StartService(i, srv, cfg); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", srv.Name, err))
		}
	}
	return errs
}

func (sm *ServerManager) StopAll() {
	sm.mu.Lock()
	procIds := make([]int, 0, len(sm.services))
	for pid := range sm.services {
		procIds = append(procIds, pid)
	}
	sm.mu.Unlock()

	for _, pid := range procIds {
		sm.StopService(pid)
	}
}

func (sm *ServerManager) GetStates() []ServiceState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	states := make([]ServiceState, 0, len(sm.services))
	for pid, rt := range sm.services {
		states = append(states, ServiceState{
			Index:  pid,
			Name:   rt.Cfg.Name,
			Status: rt.Status,
		})
	}
	return states
}

func (sm *ServerManager) GetState(procId int) ServiceState {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	rt, ok := sm.services[procId]
	if !ok {
		return ServiceState{Index: procId, Status: StatusStopped}
	}
	return ServiceState{
		Index:  procId,
		Name:   rt.Cfg.Name,
		Status: rt.Status,
	}
}

func (sm *ServerManager) startLogReader(serviceID int, cmdName string, reader io.Reader) {
	ch := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		close(ch)
	}()
	go func() {
		for line := range ch {
			if sm.onLog != nil {
				sm.onLog(serviceID, cmdName, line)
			}
		}
	}()
}

func (sm *ServerManager) waitForCompletion(procId int, rt *ServiceRuntime) {
	var wg sync.WaitGroup
	for _, cmd := range rt.Cmds {
		wg.Add(1)
		go func(c *exec.Cmd) {
			defer wg.Done()
			c.Wait()
		}(cmd)
	}
	wg.Wait()

	sm.mu.Lock()
	alreadyStopped := rt.Status == StatusStopped || rt.Status == StatusError
	sm.mu.Unlock()

	if !alreadyStopped {
		sm.StopService(procId)
	} else if sm.onStateChange != nil {
		sm.onStateChange()
	}
}
