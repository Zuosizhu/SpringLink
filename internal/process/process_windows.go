//go:build windows

package process

import (
	"os/exec"
	"strconv"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	tk := exec.Command("taskkill", "/F", "/T", "/PID",
		strconv.Itoa(cmd.Process.Pid))
	tk.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	tk.Run()
}
