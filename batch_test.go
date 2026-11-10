package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFake 写一个假命令行工具，body 是 sh 脚本正文。
func writeFake(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func mustPDF(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// 两个不同目录下的同名 PDF，输出到同一目录时不能互相覆盖。
func TestBatchSameBaseNameNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pdftoppm", `
prefix=""
for a in "$@"; do prefix="$a"; done
for i in 1 2; do echo x > "${prefix}-0${i}.png"; done
`)
	pdfinfo := writeFake(t, dir, "pdfinfo", `echo "Pages:          2"`)
	_ = pdfinfo
	t.Setenv("PDF2IMG_PDFTOPPM", filepath.Join(dir, "pdftoppm"))
	t.Setenv("PDF2IMG_PDFINFO", filepath.Join(dir, "pdfinfo"))

	a := filepath.Join(dir, "a", "日报.pdf")
	b := filepath.Join(dir, "b", "日报.pdf")
	mustPDF(t, a)
	mustPDF(t, b)

	out := filepath.Join(dir, "out")
	opt := Options{DPI: 72, Format: "png", Quality: 92, NamePattern: "{name}_{page}"}
	sum := convertBatch([]string{a, b}, out, opt, nil, &convReporter{}, 2)

	if sum.OK != 2 || sum.Failed != 0 || sum.Images != 4 {
		t.Fatalf("汇总不对：%+v", sum)
	}
	entries, _ := os.ReadDir(out)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 4 {
		t.Fatalf("期望 4 张图片，实际 %d 张：%v", len(names), names)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"日报_001.png", "日报_002.png", "日报 (2)_001.png", "日报 (2)_002.png"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("缺少 %s，实际：%v", want, names)
		}
	}
}

// 命名模板里没有 {page} 时，多页 PDF 不能全部写成同一个文件。
func TestPatternWithoutPageKeepsEveryPage(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pdftoppm", `
prefix=""
for a in "$@"; do prefix="$a"; done
for i in 1 2 3; do echo x > "${prefix}-0${i}.png"; done
`)
	writeFake(t, dir, "pdfinfo", `echo "Pages:          3"`)
	t.Setenv("PDF2IMG_PDFTOPPM", filepath.Join(dir, "pdftoppm"))
	t.Setenv("PDF2IMG_PDFINFO", filepath.Join(dir, "pdfinfo"))

	pdf := filepath.Join(dir, "x.pdf")
	mustPDF(t, pdf)
	out := filepath.Join(dir, "out")
	res := convertOne(pdf, out, Options{DPI: 72, Format: "png", Quality: 92, NamePattern: "{name}"}, nil, &convReporter{})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	entries, _ := os.ReadDir(out)
	if len(entries) != 3 {
		t.Fatalf("期望 3 张图片，实际 %d（模板缺 {page} 时被覆盖了）", len(entries))
	}
	if res.Written != 3 {
		t.Fatalf("Written=%d，期望 3", res.Written)
	}
}

// 多段页码范围 + 跳过已存在：第一段被跳过的临时文件不能在第二段再计一次。
func TestSkipExistsNotDoubleCountedAcrossRanges(t *testing.T) {
	dir := t.TempDir()
	// 假 pdftoppm 会按 -f/-l 生成对应页码，模拟真实分段输出
	writeFake(t, dir, "pdftoppm", `
first=1
last=0
prev=""
for a in "$@"; do
  case "$prev" in
    -f) first="$a" ;;
    -l) last="$a" ;;
  esac
  prev="$a"
  prefix="$a"
done
if [ "$last" = "0" ]; then last="$first"; fi
i=$first
while [ "$i" -le "$last" ]; do
  echo x > "$(printf '%s-%02d.png' "$prefix" "$i")"
  i=$((i+1))
done
`)
	writeFake(t, dir, "pdfinfo", `echo "Pages:          4"`)
	t.Setenv("PDF2IMG_PDFTOPPM", filepath.Join(dir, "pdftoppm"))
	t.Setenv("PDF2IMG_PDFINFO", filepath.Join(dir, "pdfinfo"))

	pdf := filepath.Join(dir, "y.pdf")
	mustPDF(t, pdf)
	out := filepath.Join(dir, "out")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	// 预置第 1、2 页成品，两个区间里的前一段会跳过它们
	for _, n := range []string{"y_001.png", "y_002.png"} {
		if err := os.WriteFile(filepath.Join(out, n), []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	opt := Options{DPI: 72, Format: "png", Quality: 92, Pages: "1-2,3-4", SubFolder: false, SkipExists: true, NamePattern: "{name}_{page}"}
	res := convertOne(pdf, out, opt, nil, &convReporter{})
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if res.Skipped != 2 || res.Written != 2 {
		t.Fatalf("跳过数被重复累加：skipped=%d written=%d（期望 2/2）", res.Skipped, res.Written)
	}
}

// pdfinfo 读不到页数时，"5-" 只能传 -f，不能传非法的 -l 0。
func TestUnknownPageCountOmitsLastFlag(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	writeFake(t, dir, "pdftoppm", `
printf '%s\n' "$@" > "`+argsFile+`"
prefix=""
for a in "$@"; do prefix="$a"; done
echo x > "${prefix}-05.png"
`)
	writeFake(t, dir, "pdfinfo", `exit 0`)
	t.Setenv("PDF2IMG_PDFTOPPM", filepath.Join(dir, "pdftoppm"))
	t.Setenv("PDF2IMG_PDFINFO", filepath.Join(dir, "pdfinfo"))

	pdf := filepath.Join(dir, "z.pdf")
	mustPDF(t, pdf)
	res := convertOne(pdf, filepath.Join(dir, "out"),
		Options{DPI: 72, Format: "png", Quality: 92, Pages: "5-", NamePattern: "{name}_{page}"}, nil, &convReporter{})
	if res.Err != nil {
		t.Fatalf("转换失败：%v", res.Err)
	}
	blob, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	args := string(blob)
	if strings.Contains(args, "-l") {
		t.Fatalf("页数未知时不应传 -l，实际参数：\n%s", args)
	}
	if !strings.Contains(args, "-f") {
		t.Fatalf("应传 -f 5，实际参数：\n%s", args)
	}
}

// 批量并行与串行结果一致：页数、图片数相同。
func TestBatchParallelMatchesSerial(t *testing.T) {
	dir := t.TempDir()
	writeFake(t, dir, "pdftoppm", `
prefix=""
for a in "$@"; do prefix="$a"; done
for i in 1 2; do sleep 0.05; echo x > "${prefix}-0${i}.png"; done
`)
	writeFake(t, dir, "pdfinfo", `echo "Pages:          2"`)
	t.Setenv("PDF2IMG_PDFTOPPM", filepath.Join(dir, "pdftoppm"))
	t.Setenv("PDF2IMG_PDFINFO", filepath.Join(dir, "pdfinfo"))

	var pdfs []string
	for i, n := range []string{"a.pdf", "b.pdf", "c.pdf", "d.pdf"} {
		p := filepath.Join(dir, "in"+string(rune('0'+i)), n)
		mustPDF(t, p)
		pdfs = append(pdfs, p)
	}
	opt := Options{DPI: 72, Format: "png", Quality: 92, SubFolder: true, NamePattern: "{name}_{page}"}
	serial := convertBatch(pdfs, filepath.Join(dir, "s"), opt, nil, &convReporter{}, 1)
	parallel := convertBatch(pdfs, filepath.Join(dir, "p"), opt, nil, &convReporter{}, 4)
	if serial.OK != parallel.OK || serial.Images != parallel.Images || serial.Failed != 0 || parallel.Failed != 0 {
		t.Fatalf("并行与串行结果不一致：serial=%+v parallel=%+v", serial, parallel)
	}
}
