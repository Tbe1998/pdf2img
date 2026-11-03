//go:build !windows

package main

import "fmt"

func runInstaller() {}

func runGUI() error {
	return fmt.Errorf("图形界面仅在 Windows 下可用，请用 --cli 模式")
}
