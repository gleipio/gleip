//go:build unix

package backend

import (
	"os/exec"
	"syscall"
)

// setProcAttributes sets process attributes for Unix systems
func setProcAttributes(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Detach the process
	}
} 