package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// runSelfTest 生成一个最小 PDF，转换后验证产物存在。
func runSelfTest(console bool) bool {
	logf := func(s string) {
		if console {
			fmt.Println(s)
		}
	}
	logf("开始自检...")
	tmp, err := os.MkdirTemp("", "pdf2img-selftest-")
	if err != nil {
		logf("失败：创建临时目录失败 " + err.Error())
		return false
	}
	defer os.RemoveAll(tmp)

	pdf := filepath.Join(tmp, "test.pdf")
	if err := writeTinyPDF(pdf); err != nil {
		logf("失败：写入测试 PDF 失败 " + err.Error())
		return false
	}
	logf("测试 PDF 已写入")

	out := filepath.Join(tmp, "out")
	_ = os.MkdirAll(out, 0o755)
	opt := Options{DPI: 72, Format: "png", NamePattern: "{name}_{page}"}
	res := convertOne(pdf, out, opt, nil, &convReporter{onLog: logf})
	if res.Err != nil {
		logf("失败：转换错误 " + res.Err.Error())
		return false
	}
	if res.Written == 0 {
		logf("失败：没有写出任何图片")
		return false
	}
	logf(fmt.Sprintf("通过：写出 %d 张图片，页数=%d", res.Written, res.Pages))
	return true
}

// 写一个最小的 1 页 PDF：612×792 pt，带一个长方形。
func writeTinyPDF(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%%PDF-1.4\n1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	_, _ = fmt.Fprintf(f, "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	_, _ = fmt.Fprintf(f, "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /ProcSet [/PDF] >> >>\nendobj\n")
	content := "0.5 0 0 rg 100 100 400 400 re f"
	_, _ = fmt.Fprintf(f, "4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	_, _ = fmt.Fprintf(f, "xref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000296 00000 n \n")
	_, _ = fmt.Fprintf(f, "trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n461\n%%%%EOF\n")
	return f.Close()
}
