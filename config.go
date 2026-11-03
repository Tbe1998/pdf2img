package main

// 用户设置：%APPDATA%\PDF2Image\config.json（其它平台走 ~/.config）。
// 注意：输出文件夹不预填（每次自己选），但保留历史列表供“最近”按钮快选。

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	OutDir       string   `json:"out_dir"`
	DPI          int      `json:"dpi"`
	Format       string   `json:"format"`
	Quality      int      `json:"quality"`
	Pages        string   `json:"pages"`
	SubFolder    bool     `json:"sub_folder"`
	SkipExists   bool     `json:"skip_exists"`
	Gray         bool     `json:"gray"`
	OpenWhenDone bool     `json:"open_when_done"`
	NamePattern  string   `json:"name_pattern"`
	AutoCheckUpd bool     `json:"auto_check_update"`
	RecentOut    []string `json:"recent_out,omitempty"`
}

func defaultConfig() Config {
	return Config{
		DPI:          200,
		Format:       "png",
		Quality:      92,
		SubFolder:    true,
		OpenWhenDone: true,
		NamePattern:  "{name}_{page}",
		AutoCheckUpd: true,
	}
}

func (c Config) options() Options {
	return Options{
		DPI:         c.DPI,
		Format:      c.Format,
		Quality:     c.Quality,
		Pages:       c.Pages,
		SubFolder:   c.SubFolder,
		SkipExists:  c.SkipExists,
		Gray:        c.Gray,
		NamePattern: c.NamePattern,
	}
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	d := filepath.Join(base, "PDF2Image")
	_ = os.MkdirAll(d, 0o755)
	return d
}

func configPath() string  { return filepath.Join(configDir(), "config.json") }
func logFilePath() string { return filepath.Join(configDir(), "app.log") }

func loadConfig() Config {
	cfg := defaultConfig()
	if b, err := os.ReadFile(configPath()); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	if cfg.DPI < 36 || cfg.DPI > 1200 {
		cfg.DPI = 200
	}
	if cfg.Format == "" {
		cfg.Format = "png"
	}
	if cfg.Quality < 30 || cfg.Quality > 100 {
		cfg.Quality = 92
	}
	if cfg.NamePattern == "" {
		cfg.NamePattern = "{name}_{page}"
	}
	if len(cfg.RecentOut) > 10 {
		cfg.RecentOut = cfg.RecentOut[:10]
	}
	return cfg
}

func (c Config) save() error {
	if err := os.MkdirAll(configDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, configPath())
}

// rememberOut 把输出目录放进历史：最近优先、去重、上限 10 条。
func (c *Config) rememberOut(dir string) {
	dir = filepath.Clean(dir)
	if dir == "" || dir == "." {
		return
	}
	out := []string{dir}
	for _, d := range c.RecentOut {
		if !samePath(d, dir) {
			out = append(out, d)
		}
	}
	if len(out) > 10 {
		out = out[:10]
	}
	c.RecentOut = out
}

func samePath(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return a == b
	}
	if os.PathSeparator == '\\' {
		return pathEqualFold(aa, bb)
	}
	return aa == bb
}

func pathEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func appendLogFile(line string) {
	p := logFilePath()
	if st, err := os.Stat(p); err == nil && st.Size() > 1<<20 {
		_ = os.WriteFile(p, []byte(""), 0o644)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\r\n")
}
