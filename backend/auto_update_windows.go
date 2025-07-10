//go:build windows

package backend

import (
	"os/exec"
)

// setProcAttributes sets process attributes for Windows systems
// Windows doesn't support Setpgid, so this is a no-op
func setProcAttributes(cmd *exec.Cmd) {
	// No-op on Windows
} 