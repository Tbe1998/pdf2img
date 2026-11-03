package main

import (
	"fmt"
	"os"
	"strings"
)

func runCLI(args []string) error {
	var pdfs []string
	opt := defaultConfig().options()
	var outDir string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--cli":
			continue
		case strings.HasPrefix(a, "--out="):
			outDir = strings.TrimPrefix(a, "--out=")
		case a == "--out" && i+1 < len(args):
			outDir = args[i+1]; i++
		case strings.HasPrefix(a, "--dpi="):
			opt.DPI = atoiSafe(strings.TrimPrefix(a, "--dpi="))
		case a == "--dpi" && i+1 < len(args):
			opt.DPI = atoiSafe(args[i+1]); i++
		case strings.HasPrefix(a, "--format="):
			opt.Format = strings.TrimPrefix(a, "--format=")
		case a == "--format" && i+1 < len(args):
			opt.Format = args[i+1]; i++
		case strings.HasPrefix(a, "--pages="):
			opt.Pages = strings.TrimPrefix(a, "--pages=")
		case a == "--pages" && i+1 < len(args):
			opt.Pages = args[i+1]; i++
		case strings.HasPrefix(a, "--password="):
			opt.Password = strings.TrimPrefix(a, "--password=")
		case a == "--password" && i+1 < len(args):
			opt.Password = args[i+1]; i++
		case a == "--subfolder":
			opt.SubFolder = true
		case a == "--skip-existing":
			opt.SkipExists = true
		case a == "--gray":
			opt.Gray = true
		case strings.HasPrefix(a, "--pattern="):
			opt.NamePattern = strings.TrimPrefix(a, "--pattern=")
		case a == "--pattern" && i+1 < len(args):
			opt.NamePattern = args[i+1]; i++
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("未知参数 %s", a)
		default:
			pdfs = append(pdfs, a)
		}
	}
	if outDir == "" {
		return fmt.Errorf("请用 --out 指定输出目录")
	}
	if len(pdfs) == 0 {
		return fmt.Errorf("请给出 PDF 文件路径")
	}
	files, err := collectPDFs(pdfs, false)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("没有找到 PDF 文件")
	}
	fmt.Printf("找到 %d 个 PDF，输出到 %s\n", len(files), outDir)
	totalPages := 0
	for _, pdf := range files {
		tool, _ := findTool("pdfinfo")
		if tool != "" {
			if n, e := pdfPageCount(tool, pdf, opt.Password); e == nil {
				totalPages += n
			}
		}
	}
	done := 0
	for _, pdf := range files {
		res := convertOne(pdf, outDir, opt, nil, &convReporter{
			onLog: func(s string) { fmt.Println(s) },
			onAdd: func(n int) {
				done += n
				fmt.Printf("  进度 %d/%d\n", done, totalPages)
			},
		})
		if res.Err != nil {
			fmt.Fprintf(os.Stderr, "错误：%s -> %v\n", res.PDF, res.Err)
		}
		fmt.Printf("完成 %s: 共 %d 页，写出 %d 页，跳过 %d 页\n", res.PDF, res.Pages, res.Written, res.Skipped)
	}
	return nil
}
