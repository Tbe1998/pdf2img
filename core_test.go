package main

import "testing"

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
