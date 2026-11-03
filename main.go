package main

import (
	"fmt"
	"os"
	"runtime"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "panic: %v\n", r)
			os.Exit(2)
		}
	}()

	if runtime.GOOS == "windows" {
		// 安装器独立处理，不走其它逻辑。
		for _, a := range os.Args[1:] {
			if a == "--install-update" {
				runInstaller()
				return
			}
		}
	}

	// 命令行模式
	if hasArg(os.Args[1:], "--cli") {
		if err := runCLI(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if hasArg(os.Args[1:], "--selftest") {
		console := hasArg(os.Args[1:], "--selftest-console")
		ok := runSelfTest(console)
		if !ok {
			os.Exit(1)
		}
		return
	}
	if hasArg(os.Args[1:], "--checkupdate") {
		if err := checkUpdate(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// GUI
	if err := runGUI(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func hasArg(args []string, key string) bool {
	for _, a := range args {
		if a == key {
			return true
		}
	}
	return false
}

func atoiSafe(s string) int {
	v, _ := parseInt(s)
	return v
}

func parseInt(s string) (int, error) {
	return atoi(s)
}
