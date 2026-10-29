//go:build !darwin && !linux

package gateway

import "os/exec"

func detach(c *exec.Cmd) {}
