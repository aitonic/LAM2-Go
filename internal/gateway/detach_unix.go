//go:build darwin || linux

package gateway

import (
	"os/exec"
	"syscall"
)

func detach(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
