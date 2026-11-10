package main

// 核心转换逻辑：与界面无关，也不含任何 build tag，
// 这样在 Linux 上就能用单元测试 + 假 pdftoppm 把参数拼装、命名、清理全部跑通。

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const appVersion = "1.1.0"

// 内置更新地址（界面不显示、不可修改）。
const defaultUpdateURL = "https://www.topbei.cn/pdf2img/latest.json"

// Options 是一次转换的全部参数。
type Options struct {
	DPI         int    // 72~1200
	Format      string // png / jpg
	Quality     int    // JPEG 质量 1~100
	Pages       string // "" = 全部；"1-3,5"
	Password    string // PDF 打开密码（-upw）
	SubFolder   bool   // 每个 PDF 单独建子文件夹
	SkipExists  bool   // 同名图片已存在则跳过
	Gray        bool   // 灰度
	NamePattern string // 输出文件名模板，{name} {page}
}

func (o *Options) normalize() {
	if o.DPI < 36 {
		o.DPI = 200
	}
	if o.DPI > 1200 {
		o.DPI = 1200
	}
	switch strings.ToLower(strings.TrimSpace(o.Format)) {
	case "jpg", "jpeg":
		o.Format = "jpg"
	default:
		o.Format = "png"
	}
	if o.Quality < 30 || o.Quality > 100 {
		o.Quality = 92
	}
	if strings.TrimSpace(o.NamePattern) == "" {
		o.NamePattern = "{name}_{page}"
	}
}

// 页码范围 -------------------------------------------------------------

type pageRange struct{ first, last int } // 1-based，含两端；last=0 表示到末尾

var rangeItemRe = regexp.MustCompile(`^\s*(\d+)\s*(?:[-~—]\s*(\d+)?)?\s*$`)

// parsePageRanges 把 "1-3,5" 解析成 [{1,3},{5,5}]；
// 空字符串表示整本（返回 nil，调用方按整本处理）。
func parsePageRanges(spec string) ([]pageRange, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var out []pageRange
	for _, part := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == '，' || r == ';' || r == '；' }) {
		if strings.TrimSpace(part) == "" {
			continue
		}
		m := rangeItemRe.FindStringSubmatch(part)
		if m == nil {
			return nil, fmt.Errorf("页码范围看不懂：%q（正确写法：1-3,5）", part)
		}
		a, _ := strconv.Atoi(m[1])
		if a <= 0 {
			return nil, fmt.Errorf("页码要从 1 开始：%q", part)
		}
		b := a
		if m[2] != "" {
			b, _ = strconv.Atoi(m[2])
		} else if strings.Contains(part, "-") || strings.Contains(part, "~") || strings.Contains(part, "—") {
			b = 0 // "5-" 表示第 5 页到末尾
		}
		if b != 0 && b < a {
			return nil, fmt.Errorf("页码范围反了：%q", part)
		}
		out = append(out, pageRange{a, b})
	}
	if len(out) == 0 {
		return nil, errors.New("页码范围是空的")
	}
	return out, nil
}

// 工具定位 -------------------------------------------------------------

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// findTool 依次在：环境变量覆盖 → 程序同目录\bin → 程序同目录 → PATH 里找。
func findTool(name string) (string, error) {
	envKey := "PDF2IMG_" + strings.ToUpper(name)
	if p := strings.TrimSpace(os.Getenv(envKey)); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	exeName := name + exeSuffix()
	var bases []string
	if self, err := os.Executable(); err == nil {
		dir := filepath.Dir(self)
		bases = append(bases, filepath.Join(dir, "bin"), dir)
	}
	bases = append(bases, ".")
	for _, b := range bases {
		p := filepath.Join(b, exeName)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath(exeName); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("找不到 %s（程序目录\\bin 和系统 PATH 里都没有）", exeName)
}

// PDF 信息 -------------------------------------------------------------

// pdfPageCount 用 pdfinfo 读页数；读不到就返回 0，让调用方按需回退。
func pdfPageCount(pdfinfo, pdf, password string) (int, error) {
	args := []string{}
	if password != "" {
		args = append(args, "-upw", password)
	}
	args = append(args, pdf)
	cmd := exec.Command(pdfinfo, args...)
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg := strings.TrimSpace(string(ee.Stderr))
			if msg == "" {
				msg = "pdfinfo 读取失败"
			}
			return 0, errors.New(cleanToolError(msg))
		}
		return 0, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "Pages:") {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "Pages:")))
			return n, nil
		}
	}
	return 0, errors.New("pdfinfo 没有返回页数")
}

// cleanToolError 把命令行工具的英文报错翻译成人话，保留原文便于排查。
func cleanToolError(msg string) string {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "incorrect password") || strings.Contains(low, "password"):
		return "PDF 有打开密码，请在“PDF 密码”里填写后再转（" + firstLine(msg) + "）"
	case strings.Contains(low, "couldn't open") || strings.Contains(low, "no such file"):
		return "文件打不开，可能路径不存在或文件被占用（" + firstLine(msg) + "）"
	case strings.Contains(low, "may not be a pdf") || strings.Contains(low, "damaged"):
		return "这个文件不是有效的 PDF，或者已经损坏（" + firstLine(msg) + "）"
	}
	return firstLine(msg)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// 文件收集 -------------------------------------------------------------

// collectPDFs 把用户拖进来/选中的文件与文件夹展开成去重后的 PDF 绝对路径列表。
func collectPDFs(paths []string, recursive bool) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		key := strings.ToLower(abs)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, abs)
	}
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			if recursive {
				_ = filepath.Walk(p, func(q string, info os.FileInfo, err error) error {
					if err != nil || info == nil || info.IsDir() {
						return nil
					}
					if strings.EqualFold(filepath.Ext(q), ".pdf") {
						add(q)
					}
					return nil
				})
			} else {
				ents, _ := os.ReadDir(p)
				for _, e := range ents {
					if e.IsDir() {
						continue
					}
					if strings.EqualFold(filepath.Ext(e.Name()), ".pdf") {
						add(filepath.Join(p, e.Name()))
					}
				}
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(p), ".pdf") {
			add(p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out, nil
}

var illegalName = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

// safeBase 把 PDF 文件名变成可以当文件名用的前缀。
func safeBase(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = illegalName.ReplaceAllString(name, "_")
	name = strings.Trim(name, " .")
	if name == "" {
		name = "page"
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	return name
}

// 命名模板 -------------------------------------------------------------

func outName(pattern, base string, page, width int) string {
	s := strings.ReplaceAll(pattern, "{name}", base)
	s = strings.ReplaceAll(s, "{page}", fmt.Sprintf("%0*d", width, page))
	return illegalName.ReplaceAllString(s, "_")
}

// 转换 -----------------------------------------------------------------

// Result 是单个 PDF 的转换结果。
type Result struct {
	PDF     string
	OutDir  string
	Pages   int
	Written int
	Skipped int
	Err     error
}

type convReporter struct {
	mu     sync.Mutex
	onLog  func(string)
	onAdd  func(int)    // 每完成一页 +1
	onDone func(Result) // 每个 PDF 转换结束（含失败）
}

func (r *convReporter) logf(format string, a ...any) {
	if r.onLog != nil {
		// 多个转换协程会并发写日志，串行化避免半行交错
		r.mu.Lock()
		defer r.mu.Unlock()
		r.onLog(fmt.Sprintf(format, a...))
	}
}
func (r *convReporter) pageDone() { r.pagesDone(1) }

// pagesDone 一次上报 n 页（轮询到新产物时批量上报，避免逐页抢锁）。
func (r *convReporter) pagesDone(n int) {
	if r.onAdd != nil && n > 0 {
		r.mu.Lock()
		r.onAdd(n)
		r.mu.Unlock()
	}
}

// nameAlloc 在批量转换时给输出文件占位，防止两个不同目录下的同名 PDF
// 同时写同一个输出文件名（后写的会把前一个静默覆盖）。
type nameAlloc struct {
	mu    sync.Mutex
	taken map[string]bool
}

func newNameAlloc() *nameAlloc {
	return &nameAlloc{taken: map[string]bool{}}
}

// claim 返回本次可安全写入的路径：同名冲突时追加 " (2)"、" (3)"…
func (a *nameAlloc) claim(p string) string {
	if a == nil {
		return p
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := strings.ToLower(p)
	if !a.taken[key] {
		a.taken[key] = true
		return p
	}
	ext := filepath.Ext(p)
	stem := strings.TrimSuffix(p, ext)
	for i := 2; i <= 999; i++ {
		cand := fmt.Sprintf("%s (%d)%s", stem, i, ext)
		ck := strings.ToLower(cand)
		if !a.taken[ck] {
			a.taken[ck] = true
			return cand
		}
	}
	return p
}

// convertOne 转换一个 PDF。所有输出先写到临时目录，成功后再改名搬到目标目录，
// 中途失败不会留下半套图片。
func convertOne(pdf, outRoot string, opt Options, cancel *cancelToken, rep *convReporter) Result {
	return convertOneWith(pdf, outRoot, opt, cancel, rep, nil, "")
}

// convertOneWith 同 convertOne，额外接受批量输出名占位器（可为 nil）与调用方指定的文件名前缀。
func convertOneWith(pdf, outRoot string, opt Options, cancel *cancelToken, rep *convReporter, alloc *nameAlloc, baseOverride string) Result {
	opt.normalize()
	res := Result{PDF: pdf}
	base := safeBase(filepath.Base(pdf))
	if baseOverride != "" {
		base = baseOverride
	}
	res.OutDir = outRoot
	target := outRoot
	if opt.SubFolder {
		target = filepath.Join(outRoot, base)
		res.OutDir = target
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		res.Err = fmt.Errorf("创建输出文件夹失败：%v", err)
		return res
	}

	toolDir, err := findTool("pdftoppm")
	if err != nil {
		res.Err = err
		return res
	}
	infoTool, _ := findTool("pdfinfo")

	total := 0
	if infoTool != "" {
		if n, e := pdfPageCount(infoTool, pdf, opt.Password); e == nil {
			total = n
		}
	}
	res.Pages = total

	ranges, err := parsePageRanges(opt.Pages)
	if err != nil {
		res.Err = err
		return res
	}

	// 命名模板里没有 {page} 时，多页输出会全部写成同一个文件名，
	// 最终只剩最后一页（旧版本的静默丢页 bug）。页数不止 1 页就自动补页码。
	pattern := opt.NamePattern
	if !strings.Contains(pattern, "{page}") && expectedPages(total, ranges) != 1 {
		pattern += "_{page}"
	}

	tmp, err := os.MkdirTemp(target, ".pdf2img-")
	if err != nil {
		res.Err = fmt.Errorf("创建临时目录失败：%v", err)
		return res
	}
	defer os.RemoveAll(tmp)

	ext := "." + opt.Format
	width := 3
	if total > 999 {
		width = len(strconv.Itoa(total))
	}

	// 需要处理的区间
	type seg struct{ first, last int }
	var segs []seg
	if ranges == nil {
		segs = []seg{{0, 0}}
	} else {
		for _, r := range ranges {
			last := r.last
			if last == 0 {
				// 页数未知时 last 保持 0，下面只传 -f 不传 -l（pdftoppm 会转到末尾）；
				// 直接传 -l 0 是非法页码，会让整段转换失败。
				last = total
			}
			segs = append(segs, seg{r.first, last})
		}
	}

	for _, sg := range segs {
		if cancel != nil && cancel.stopped() {
			res.Err = errStopped
			return res
		}
		prefix := filepath.Join(tmp, "p")
		args := []string{"-r", strconv.Itoa(opt.DPI)}
		if opt.Format == "jpg" {
			args = append(args, "-jpeg", "-jpegopt", "quality="+strconv.Itoa(opt.Quality))
		} else {
			args = append(args, "-png")
		}
		if opt.Gray {
			args = append(args, "-gray")
		}
		if opt.Password != "" {
			args = append(args, "-upw", opt.Password)
		}
		if sg.first > 0 {
			args = append(args, "-f", strconv.Itoa(sg.first))
			if sg.last > 0 {
				args = append(args, "-l", strconv.Itoa(sg.last))
			}
		}
		args = append(args, pdf, prefix)

		cmd := exec.Command(toolDir, args...)
		hideWindow(cmd)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		rep.logf("  转换：%s  %s dpi%s", filepath.Base(pdf), strconv.Itoa(opt.DPI), pageHint(sg.first, sg.last))
		if err := cmd.Start(); err != nil {
			res.Err = fmt.Errorf("调用 pdftoppm 失败：%v", err)
			return res
		}
		if cancel != nil {
			cancel.attach(cmd)
		}

		// 边转边报进度：pdftoppm 每写出一页就往临时目录丢一个文件，
		// 每 150ms 数一次新增文件数并上报，用户能实时看到“8/40 页”。
		// 不能等 cmd.Wait() 再数——那时已经全部转完，进度会直接 0→100。
		doneCh := make(chan error, 1)
		go func() { doneCh <- cmd.Wait() }()
		segReported := 0
		countNow := func() int {
			files, _ := filepath.Glob(filepath.Join(tmp, "p-*"+ext))
			return len(files)
		}
		werr := error(nil)
		tick := time.NewTicker(150 * time.Millisecond)
	waitLoop:
		for {
			select {
			case werr = <-doneCh:
				if n := countNow(); n > segReported {
					rep.pagesDone(n - segReported)
					segReported = n
				}
				break waitLoop
			case <-tick.C:
				if n := countNow(); n > segReported {
					rep.pagesDone(n - segReported)
					segReported = n
				}
			}
		}
		tick.Stop()
		if cancel != nil {
			cancel.detach(cmd)
		}
		if cancel != nil && cancel.stopped() {
			res.Err = errStopped
			return res
		}
		if werr != nil {
			res.Err = errors.New(cleanToolError(stderr.String()))
			return res
		}

		files, _ := filepath.Glob(filepath.Join(tmp, "p-*"+ext))
		sort.Strings(files)
		for _, f := range files {
			page := pageFromTempName(f)
			if page <= 0 {
				continue
			}
			dst := alloc.claim(filepath.Join(target, outName(pattern, base, page, width)+ext))
			if opt.SkipExists {
				if _, err := os.Stat(dst); err == nil {
					res.Skipped++
					// 必须删掉临时产物：否则下一个页码区间会再次统计同一页，
					// 跳过数被重复累加，还会残留在临时目录里直到整个任务结束。
					_ = os.Remove(f)
					continue
				}
			}
			if err := os.Rename(f, dst); err != nil {
				// 跨设备时退回复制
				if cerr := copyFile(f, dst); cerr != nil {
					res.Err = fmt.Errorf("写出图片失败：%v", cerr)
					return res
				}
			}
			res.Written++
		}
	}
	return res
}

// expectedPages 算出这次实际会转多少页：没填范围=整本，填了范围=各区间页数之和。
func expectedPages(total int, ranges []pageRange) int {
	if total <= 0 {
		return 0
	}
	if ranges == nil {
		return total
	}
	sum := 0
	for _, r := range ranges {
		last := r.last
		if last == 0 || last > total {
			last = total
		}
		if r.first <= total && last >= r.first {
			sum += last - r.first + 1
		}
	}
	return sum
}

func pageHint(first, last int) string {
	if first == 0 {
		return ""
	}
	if last <= first {
		return fmt.Sprintf("（第 %d 页）", first)
	}
	return fmt.Sprintf("（第 %d-%d 页）", first, last)
}

var tempPageRe = regexp.MustCompile(`-(\d+)$`)

func pageFromTempName(p string) int {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	m := tempPageRe.FindStringSubmatch(base)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// cancelToken 让"停止"能杀掉正在跑的 pdftoppm。
type cancelToken struct {
	mu      sync.Mutex
	flag    bool
	procs   map[*exec.Cmd]bool
	errOnce sync.Once
}

func newCancelToken() *cancelToken { return &cancelToken{procs: map[*exec.Cmd]bool{}} }

func (c *cancelToken) stop() {
	c.mu.Lock()
	c.flag = true
	for p := range c.procs {
		if p.Process != nil {
			_ = p.Process.Kill()
		}
	}
	c.mu.Unlock()
}

func (c *cancelToken) stopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flag
}

func (c *cancelToken) attach(cmd *exec.Cmd) {
	c.mu.Lock()
	c.procs[cmd] = true
	if c.flag && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	c.mu.Unlock()
}

func (c *cancelToken) detach(cmd *exec.Cmd) {
	c.mu.Lock()
	delete(c.procs, cmd)
	c.mu.Unlock()
}

var errStopped = errors.New("已停止")

// BatchSummary 是一次批量转换的汇总。
type BatchSummary struct {
	Files   int
	OK      int
	Failed  int
	Images  int
	Skipped int
}

// convertBatch 并行转换多个 PDF（jobs 个协程，1~8）。
// 输出文件名在启动前统一去重，保证两个不同目录下的同名 PDF 不会互相覆盖，
// 且文件名分配结果与串行执行一致（按传入顺序先到先得）。
func convertBatch(pdfs []string, outRoot string, opt Options, cancel *cancelToken, rep *convReporter, jobs int) BatchSummary {
	var sum BatchSummary
	if len(pdfs) == 0 {
		return sum
	}
	opt.normalize()
	if jobs < 1 {
		jobs = 1
	}
	if jobs > 8 {
		jobs = 8
	}
	if jobs > len(pdfs) {
		jobs = len(pdfs)
	}

	// 先串行分配文件名前缀：同名 PDF 依次得到 "名字"、"名字 (2)"…
	alloc := newNameAlloc()
	bases := make(map[string]string, len(pdfs))
	for _, pdf := range pdfs {
		b := safeBase(filepath.Base(pdf))
		if opt.SubFolder {
			// 每个 PDF 一个子文件夹，天然不冲突
			bases[pdf] = b
			continue
		}
		bases[pdf] = alloc.claimBase(outRoot, b)
	}

	results := make([]Result, len(pdfs))
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for i, pdf := range pdfs {
		if cancel != nil && cancel.stopped() {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, pdf string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := convertOneWith(pdf, outRoot, opt, cancel, rep, alloc, bases[pdf])
			results[i] = r
			if rep != nil && rep.onDone != nil {
				rep.mu.Lock()
				rep.onDone(r)
				rep.mu.Unlock()
			}
		}(i, pdf)
	}
	wg.Wait()

	sum.Files = len(pdfs)
	for _, r := range results {
		switch {
		case r.PDF == "":
			// 因停止而没跑的
		case r.Err == errStopped:
		case r.Err != nil:
			sum.Failed++
		default:
			sum.OK++
			sum.Images += r.Written
			sum.Skipped += r.Skipped
		}
	}
	return sum
}

// claimBase 给输出文件名前缀占位：同名时追加 " (2)"、" (3)"…
func (a *nameAlloc) claimBase(dir, base string) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := strings.ToLower(filepath.Join(dir, base))
	if !a.taken[key] {
		a.taken[key] = true
		return base
	}
	for i := 2; i <= 999; i++ {
		cand := fmt.Sprintf("%s (%d)", base, i)
		ck := strings.ToLower(filepath.Join(dir, cand))
		if !a.taken[ck] {
			a.taken[ck] = true
			return cand
		}
	}
	return base
}

// copyFile 复制文件；任何一步失败都会删掉目标，避免留下半截图片。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		_ = os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
