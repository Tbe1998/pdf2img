//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindow 让 pdftoppm 之类的控制台程序不弹黑框。
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
	cmd.SysProcAttr.HideWindow = true
}
