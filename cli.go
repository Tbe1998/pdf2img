package main

import (
	"fmt"
	"strings"
	"time"
)

func runCLI(args []string) error {
	var pdfs []string
	opt := defaultConfig().options()
	var outDir string
	jobs := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--cli":
			continue
		case strings.HasPrefix(a, "--out="):
			outDir = strings.TrimPrefix(a, "--out=")
		case a == "--out" && i+1 < len(args):
			outDir = args[i+1]
			i++
		case strings.HasPrefix(a, "--jobs="):
			jobs = atoiSafe(strings.TrimPrefix(a, "--jobs="))
		case a == "--jobs" && i+1 < len(args):
			jobs = atoiSafe(args[i+1])
			i++
		case strings.HasPrefix(a, "--dpi="):
			opt.DPI = atoiSafe(strings.TrimPrefix(a, "--dpi="))
		case a == "--dpi" && i+1 < len(args):
			opt.DPI = atoiSafe(args[i+1])
			i++
		case strings.HasPrefix(a, "--format="):
			opt.Format = strings.TrimPrefix(a, "--format=")
		case a == "--format" && i+1 < len(args):
			opt.Format = args[i+1]
			i++
		case strings.HasPrefix(a, "--pages="):
			opt.Pages = strings.TrimPrefix(a, "--pages=")
		case a == "--pages" && i+1 < len(args):
			opt.Pages = args[i+1]
			i++
		case strings.HasPrefix(a, "--password="):
			opt.Password = strings.TrimPrefix(a, "--password=")
		case a == "--password" && i+1 < len(args):
			opt.Password = args[i+1]
			i++
		case a == "--subfolder":
			opt.SubFolder = true
		case a == "--skip-existing":
			opt.SkipExists = true
		case a == "--gray":
			opt.Gray = true
		case strings.HasPrefix(a, "--pattern="):
			opt.NamePattern = strings.TrimPrefix(a, "--pattern=")
		case a == "--pattern" && i+1 < len(args):
			opt.NamePattern = args[i+1]
			i++
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
	rep := &convReporter{
		onLog: func(s string) { fmt.Println(s) },
		onAdd: func(n int) {
			done += n
			fmt.Printf("  进度 %d/%d\n", done, totalPages)
		},
	}
	t0 := time.Now()
	sum := convertBatch(files, outDir, opt, nil, rep, jobs)
	fmt.Printf("完成：%d 个文件，成功 %d，失败 %d，写出 %d 张图片，跳过 %d 页，用时 %.1f 秒\n",
		sum.Files, sum.OK, sum.Failed, sum.Images, sum.Skipped, time.Since(t0).Seconds())
	if sum.Failed > 0 {
		return fmt.Errorf("%d 个文件转换失败", sum.Failed)
	}
	return nil
}
