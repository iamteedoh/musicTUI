//go:build windows

package audio

import "os/exec"

func setSysProcAttr(cmd *exec.Cmd) {
	// No process group isolation on Windows
}
