//go:build !windows

package main

// 非 Windows 平台（Linux 上跑单元测试 / CI）：没有窗口要隐藏。

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
