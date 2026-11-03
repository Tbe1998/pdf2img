package main

// 整包自动更新的通用部分（与 PDF 日报分拣助手同一套机制）：
// 拉取 latest.json → 比较版本 → 下载 zip → 校验大小 / sha256 → 解压 → 交给安装器。
// 更新地址写死在程序里，界面上不显示、也无法修改。

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Manifest 是 latest.json 的结构（整包更新）。
type Manifest struct {
	Version       string `json:"version"`
	Notes         string `json:"notes"`
	Package       string `json:"package"`
	PackageSize   int64  `json:"package_size"`
	PackageSHA256 string `json:"package_sha256"`
	ExeName       string `json:"exe_name"`

	// 兼容旧的单文件字段
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func (m *Manifest) isPackage() bool { return strings.TrimSpace(m.Package) != "" }

func (m *Manifest) downloadURL() string {
	if m.isPackage() {
		return m.Package
	}
	return m.URL
}

func (m *Manifest) downloadSize() int64 {
	if m.isPackage() {
		return m.PackageSize
	}
	return m.Size
}

func (m *Manifest) downloadHash() string {
	if m.isPackage() {
		return m.PackageSHA256
	}
	return m.SHA256
}

func httpClient() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

// fetchManifest 拉取 latest.json，并把相对地址解析成绝对地址。
func fetchManifest(manifestURL string) (*Manifest, error) {
	manifestURL = strings.TrimSpace(manifestURL)
	if manifestURL == "" {
		return nil, fmt.Errorf("未设置更新地址")
	}
	req, err := http.NewRequest("GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PDF2Image/"+appVersion)
	req.Header.Set("Cache-Control", "no-cache")
	resp, err := httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("服务器返回 HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("latest.json 格式不对：%v", err)
	}
	if strings.TrimSpace(m.Version) == "" {
		return nil, fmt.Errorf("latest.json 里没有 version 字段")
	}
	if strings.TrimSpace(m.downloadURL()) == "" {
		return nil, fmt.Errorf("latest.json 里没有 package 字段")
	}
	if base, err := url.Parse(manifestURL); err == nil {
		if ref, err := url.Parse(m.downloadURL()); err == nil {
			abs := base.ResolveReference(ref).String()
			if m.isPackage() {
				m.Package = abs
			} else {
				m.URL = abs
			}
		}
	}
	if !m.isPackage() {
		return nil, fmt.Errorf("这个版本的更新文件不是整包（缺少 package 字段）")
	}
	return &m, nil
}

// checkUpdate 是 --checkupdate 命令行入口（不下载，只报告）。
func checkUpdate() error {
	man, err := fetchManifest(defaultUpdateURL)
	if err != nil {
		return err
	}
	fmt.Printf("当前版本：v%s\n", appVersion)
	if compareVersion(man.Version, appVersion) <= 0 {
		fmt.Println("已是最新版本。")
		return nil
	}
	fmt.Printf("发现新版本：v%s\n", man.Version)
	if strings.TrimSpace(man.Notes) != "" {
		fmt.Println(man.Notes)
	}
	return nil
}

// compareVersion 按数字段比较，1.2.10 > 1.2.9。
func compareVersion(a, b string) int {
	pa, pb := splitVersion(a), splitVersion(b)
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			n = 0
		}
		out = append(out, n)
	}
	return out
}

// downloadPackage 下载更新包到临时目录，校验大小与 sha256。
func downloadPackage(m *Manifest) (string, error) {
	req, err := http.NewRequest("GET", m.downloadURL(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "PDF2Image/"+appVersion)
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("下载失败：HTTP %d", resp.StatusCode)
	}
	name := filepath.Base(strings.Split(m.downloadURL(), "?")[0])
	if name == "" || name == "/" || name == "." {
		name = "update.zip"
	}
	dst := filepath.Join(os.TempDir(), fmt.Sprintf("pdf2img-%s-%s", m.Version, name))
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	cerr := f.Close()
	if err != nil {
		_ = os.Remove(dst)
		return "", fmt.Errorf("下载中断：%v", err)
	}
	if cerr != nil {
		_ = os.Remove(dst)
		return "", cerr
	}
	if want := m.downloadSize(); want > 0 && n != want {
		_ = os.Remove(dst)
		return "", fmt.Errorf("下载文件大小不符（期望 %d，实际 %d）", want, n)
	}
	if want := strings.ToLower(strings.TrimSpace(m.downloadHash())); want != "" {
		if got := hex.EncodeToString(h.Sum(nil)); got != want {
			_ = os.Remove(dst)
			return "", fmt.Errorf("校验失败：sha256 不匹配")
		}
	}
	return dst, nil
}

// extractZip 把整包解压到目标目录（防目录穿越）。
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("更新包打不开：%v", err)
	}
	defer r.Close()
	cleanDest := filepath.Clean(destDir)
	if err := os.MkdirAll(cleanDest, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
			continue
		}
		target := filepath.Join(cleanDest, filepath.FromSlash(name))
		if target != cleanDest && !strings.HasPrefix(target, cleanDest+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// findExeIn 在解压目录里找主程序。
func findExeIn(dir, want string) (string, error) {
	if want != "" {
		p := filepath.Join(dir, want)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("更新包里没找到 exe")
}
