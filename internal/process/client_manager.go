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

type ClientManager struct {
	mu            sync.Mutex
	services      map[int]*ServiceRuntime
	onLog         LogCallback
	onStateChange StateChangeCallback
	clientIndex   int
	freeIndices   []int
}

func NewClientManager(onLog LogCallback, onStateChange StateChangeCallback) *ClientManager {
	return &ClientManager{
		services:      make(map[int]*ServiceRuntime),
		onLog:         onLog,
		onStateChange: onStateChange,
	}
}

func (cm *ClientManager) StartClientService(srv config.ClientServiceConfig, cfg config.Config, opts ClientOptions) (int, string, string, error) {
	result, err := orchestrator.BuildClientCommands(srv, cfg, opts.ServAddr)
	if err != nil {
		return 0, "", "", err
	}
	if len(result.Steps) == 0 {
		return 0, result.ConnAddr, "", nil
	}
	cm.mu.Lock()
	var index int
	if n := len(cm.freeIndices); n > 0 {
		index = cm.freeIndices[n-1]
		cm.freeIndices = cm.freeIndices[:n-1]
	} else {
		cm.clientIndex++
		index = cm.clientIndex
	}
	cm.mu.Unlock()
	if err := cm.startCommands(index, srv, result.Steps); err != nil {
		return 0, "", "", err
	}
	return index, "", result.LocalAddr, nil
}

func (cm *ClientManager) StartAllClients(services []config.ClientServiceConfig, cfg config.Config, opts ClientOptions) []string {
	var connAddrs []string
	for _, srv := range services {
		if !srv.Enabled {
			continue
		}
		_, connAddr, localAddr, err := cm.StartClientService(srv, cfg, opts)
		if err != nil {
			connAddrs = append(connAddrs, fmt.Sprintf("%s: %v", srv.Name, err))
		} else if localAddr != "" {
			connAddrs = append(connAddrs, fmt.Sprintf("%s: %s", srv.Name, localAddr))
		} else if connAddr != "" {
			connAddrs = append(connAddrs, fmt.Sprintf("%s: %s", srv.Name, connAddr))
		}
	}
	return connAddrs
}

func (cm *ClientManager) StopService(index int) {
	cm.mu.Lock()
	rt, ok := cm.services[index]
	if !ok {
		cm.mu.Unlock()
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
	delete(cm.services, index)
	cm.freeIndices = append(cm.freeIndices, index)
	cm.mu.Unlock()
	if cm.onStateChange != nil {
		cm.onStateChange()
	}
}

func (cm *ClientManager) StopAll() {
	cm.mu.Lock()
	indices := make([]int, 0, len(cm.services))
	for idx := range cm.services {
		indices = append(indices, idx)
	}
	cm.mu.Unlock()

	for _, idx := range indices {
		cm.StopService(idx)
	}
}

func (cm *ClientManager) GetStates() []ServiceState {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	states := make([]ServiceState, 0, len(cm.services))
	for idx, rt := range cm.services {
		states = append(states, ServiceState{
			Index:  idx,
			Name:   rt.Cfg.Name,
			Status: rt.Status,
		})
	}
	return states
}

func (cm *ClientManager) GetState(index int) ServiceState {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	rt, ok := cm.services[index]
	if !ok {
		return ServiceState{Index: index, Status: StatusStopped}
	}
	return ServiceState{
		Index:  index,
		Name:   rt.Cfg.Name,
		Status: rt.Status,
	}
}

func (cm *ClientManager) startCommands(index int, srv config.ClientServiceConfig, steps []orchestrator.Step) error {
	cm.mu.Lock()
	if rt, ok := cm.services[index]; ok && rt.Status == StatusRunning {
		cm.mu.Unlock()
		return fmt.Errorf("service %d is already running", index)
	}

	ctx, cancel := context.WithCancel(context.Background())
	rt := &ServiceRuntime{
		Cfg:    srv.ToServiceConfig(),
		Steps:  steps,
		Status: StatusStarting,
		Cancel: cancel,
	}
	cm.services[index] = rt
	cm.mu.Unlock()

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

		if cm.onLog != nil {
			cm.onLog(index, cc.Name, fmt.Sprintf("启动命令: %s %s", cc.Program, strings.Join(cc.Args, " ")))
		}

		stdout, err := cmd.StdoutPipe()
		if err == nil {
			cm.startLogReader(index, cc.Name, stdout)
		}
		stderr, err := cmd.StderrPipe()
		if err == nil {
			cm.startLogReader(index, cc.Name, stderr)
		}

		if err := cmd.Start(); err != nil {
			cm.mu.Lock()
			for _, c := range rt.Cmds {
				killProcessTree(c)
			}
			rt.Status = StatusError
			cm.mu.Unlock()
			for i := len(steps) - 1; i >= 0; i-- {
				if steps[i].Cleanup != nil {
					steps[i].Cleanup()
				}
				for _, p := range steps[i].AllocatedPorts {
					port.Free(p)
				}
			}
			if cm.onStateChange != nil {
				cm.onStateChange()
			}
			cancel()
			return fmt.Errorf("start %s: %w", cc.Name, err)
		}

		cm.mu.Lock()
		rt.Cmds = append(rt.Cmds, cmd)
		cm.mu.Unlock()

		if step.Health != nil {
			if ok := step.Health(); !ok {
				cm.mu.Lock()
				for _, c := range rt.Cmds {
					killProcessTree(c)
				}
				rt.Status = StatusError
				cm.mu.Unlock()
				for i := len(steps) - 1; i >= 0; i-- {
					if steps[i].Cleanup != nil {
						steps[i].Cleanup()
					}
					for _, p := range steps[i].AllocatedPorts {
						port.Free(p)
					}
				}
				if cm.onStateChange != nil {
					cm.onStateChange()
				}
				cancel()
				return fmt.Errorf("health check failed for %s", step.Name)
			}
		}
	}

	cm.mu.Lock()
	rt.Status = StatusRunning
	cm.mu.Unlock()
	if cm.onStateChange != nil {
		cm.onStateChange()
	}

	go cm.waitForCompletion(index, rt)
	return nil
}

func (cm *ClientManager) startLogReader(serviceID int, cmdName string, reader io.Reader) {
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
			if cm.onLog != nil {
				cm.onLog(serviceID, cmdName, line)
			}
		}
	}()
}

func (cm *ClientManager) waitForCompletion(index int, rt *ServiceRuntime) {
	var wg sync.WaitGroup
	for _, cmd := range rt.Cmds {
		wg.Add(1)
		go func(c *exec.Cmd) {
			defer wg.Done()
			c.Wait()
		}(cmd)
	}
	wg.Wait()

	cm.mu.Lock()
	alreadyStopped := rt.Status == StatusStopped || rt.Status == StatusError
	cm.mu.Unlock()

	if !alreadyStopped {
		cm.StopService(index)
	} else if cm.onStateChange != nil {
		cm.onStateChange()
	}
}
