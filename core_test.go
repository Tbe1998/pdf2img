package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.10", -1},
		{"v1.2", "1.1.9", 1},
		{"1.2.0-beta", "1.2.0", 0},
		{"", "1.0.0", -1},
	}
	for _, c := range cases {
		if got := compareVersion(c.a, c.b); got != c.want {
			t.Fatalf("compareVersion(%q,%q)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParsePageRanges(t *testing.T) {
	ok := []string{"1", "1-3", "1-3,5", "1，2，5-7", "4-", "2~5", " 3 "}
	for _, s := range ok {
		if _, err := parsePageRanges(s); err != nil {
			t.Fatalf("parsePageRanges(%q) 应当接受，却报错：%v", s, err)
		}
	}
	bad := []string{"abc", "0", "5-2", "1--3", "-2"}
	for _, s := range bad {
		if _, err := parsePageRanges(s); err == nil {
			t.Fatalf("parsePageRanges(%q) 应当报错，却通过了", s)
		}
	}
	if r, err := parsePageRanges("5-"); err != nil || len(r) != 1 || r[0].first != 5 || r[0].last != 0 {
		t.Fatalf("5- 应当解析为 5 到末尾：%+v %v", r, err)
	}
}

func TestOutName(t *testing.T) {
	cases := []struct {
		pattern, base string
		page, width   int
		want          string
	}{
		{"{name}_{page}", "监测日报", 3, 2, "监测日报_03"},
		{"{name}-{page}", "a/b:c", 12, 1, "a_b_c-12"},
		{"第{page}页", "x", 7, 3, "第007页"},
	}
	for _, c := range cases {
		if got := outName(c.pattern, c.base, c.page, c.width); got != c.want {
			t.Fatalf("outName=%q, want %q", got, c.want)
		}
	}
}

func TestNormalizeOptions(t *testing.T) {
	o := Options{DPI: 5, Format: "JPG", Quality: 500, NamePattern: ""}
	o.normalize()
	if o.DPI != 200 || o.Format != "jpg" || o.Quality != 92 || o.NamePattern != "{name}_{page}" {
		t.Fatalf("normalize 结果不对：%+v", o)
	}
}

func TestExpectedPages(t *testing.T) {
	cases := []struct {
		total  int
		ranges []pageRange
		want   int
	}{
		{10, nil, 10},
		{10, []pageRange{{1, 3}, {5, 5}}, 4},
		{10, []pageRange{{5, 0}}, 6},
		{10, []pageRange{{8, 99}}, 3},
		{10, []pageRange{{20, 0}}, 0},
	}
	for _, c := range cases {
		if got := expectedPages(c.total, c.ranges); got != c.want {
			t.Fatalf("expectedPages(%d,%+v)=%d, want %d", c.total, c.ranges, got, c.want)
		}
	}
}

// TestProgressReportedDuringConversion 验证进度是“边转边报”，不是等全部转完才一次性上报。
func TestProgressReportedDuringConversion(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "pdftoppm")
	script := "#!/bin/sh\n" +
		"prefix=\"\"\n" +
		"for a in \"$@\"; do prefix=\"$a\"; done\n" +
		"for i in 1 2 3 4 5 6; do sleep 0.2; echo x > \"${prefix}-0${i}.png\"; done\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PDF2IMG_PDFTOPPM", fake)

	pdf := filepath.Join(dir, "a.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	var batches []int
	var stamps []time.Duration
	rep := &convReporter{onAdd: func(n int) {
		batches = append(batches, n)
		stamps = append(stamps, time.Since(start))
	}}
	res := convertOne(pdf, filepath.Join(dir, "out"), Options{DPI: 72, Format: "png", Quality: 92, NamePattern: "{name}_{page}"}, nil, rep)
	if res.Err != nil {
		t.Fatalf("convertOne 失败：%v", res.Err)
	}
	sum := 0
	for _, n := range batches {
		sum += n
	}
	if sum != 6 || res.Written != 6 {
		t.Fatalf("上报 %d 页 / 写出 %d 页，期望 6", sum, res.Written)
	}
	if len(batches) < 3 {
		t.Fatalf("进度只上报了 %d 次（%v），说明还是集中在最后一次性上报", len(batches), batches)
	}
	if stamps[0] > 900*time.Millisecond {
		t.Fatalf("第一页在 %v 才上报，太晚了", stamps[0])
	}
	if stamps[len(stamps)-1]-stamps[0] < 400*time.Millisecond {
		t.Fatalf("全部进度集中在 %v 内报完，不是渐进式", stamps[len(stamps)-1]-stamps[0])
	}
}
