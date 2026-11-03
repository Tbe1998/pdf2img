//go:build windows

package main

// 启动追踪：GUI 程序崩溃时没有控制台输出，所以每一步写进 %TEMP%\PDF2Image-startup.log。

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

func tracePath() string {
	return filepath.Join(os.TempDir(), "PDF2Image-startup.log")
}

func trace(format string, a ...any) {
	line := time.Now().Format("2006-01-02 15:04:05.000") + "  " + fmt.Sprintf(format, a...)
	f, err := os.OpenFile(tracePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(line + "\r\n")
	_ = f.Close()
	fmt.Println(line)
}

func reportFatal(where string, r any) {
	stack := string(debug.Stack())
	msg := fmt.Sprintf("%s：%v", where, r)
	trace("！！！%s\n%s", msg, stack)
	appendLogFile("！！！" + msg + "\r\n" + stack)
	body := "程序出错了：\r\n\r\n" + msg +
		"\r\n\r\n详细信息已写入：\r\n" + tracePath() +
		"\r\n\r\n请把这个文件发给作者。"
	msgBox(0, body, "PDF 转图片助手", mbOK|mbIconWarn)
	os.Exit(1)
}
