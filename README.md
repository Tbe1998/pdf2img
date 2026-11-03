<div align="center">

<img src="design/icons/icon-256.png" width="120" alt="icon">

# PDF 转图片助手

**把 PDF 每一页批量导出成 PNG / JPG——拖进去，选目录，点开始。**

Windows 单文件 exe + 随包 poppler · 零安装 · 中文界面 · 托盘后台 · 自动更新

</div>

## 特性

- **批量转换**：一次加多个 PDF，也可以直接拖文件夹（自动递归找 PDF）
- **PNG / JPG 可选**：PNG 无损归档，JPG 可调质量、体积小
- **DPI 可调**：36~1200，默认 200
- **页码范围**：`1-3,5`、`5-`（第 5 页到末尾）、`1，2，5-7` 都认
- **加密 PDF**：填打开密码即可转
- **两种目录结构**：全部平铺，或每份 PDF 单独一个子文件夹
- **断点续转**：跳过已存在的图片
- **进度与日志**：按页统计，实时进度条；停止后已转的图片保留
- **托盘常驻**：点 × 缩到托盘，不打断手头的活
- **整包自动更新**：更新地址写死在程序里，界面不暴露、不可修改

## 为什么不用现成工具

现成的在线转换要上传文件（工程资料不适合外传），本地工具大多要装
Python 或 Office。这个程序把一个独立 exe 和 poppler 一起打包，拷到任何
Win10/Win11 机器上双击就能用，断网也能转。

## 目录结构

```
PDF2Image.exe        主程序
bin\                 随包 poppler（pdftoppm.exe / pdfinfo.exe + DLL）
使用说明.txt
```

`bin` 必须和 exe 放在一起。程序会依次在 `exe目录\bin`、`exe目录`、系统
PATH 里找 `pdftoppm.exe`，也支持环境变量 `PDF2IMG_PDFTOPPM` 指定路径。

## 命令行模式

```
PDF2Image.exe --cli 报告.pdf --out D:\图片 --dpi 300 --format jpg --pages 1-3,5
```

参数：`--dpi` `--format` `--pages` `--password` `--subfolder`
`--skip-existing` `--gray` `--pattern "{name}_{page}"`

自检（造一张测试 PDF 并真转成图片）：

```
PDF2Image.exe --selftest-console
```

检查更新（只报告，不下载）：

```
PDF2Image.exe --checkupdate
```

## 构建

本地：装 Go 1.22+，执行 `build.bat`（需要自己准备 `bin` 里的 poppler）。

CI：推 tag 触发 GitHub Actions，在 Windows 上自动下载并裁剪 poppler、
编译、跑自检、打包 zip 并挂到 Release。

## 发版

1. 改 `core.go` 里的 `const appVersion`
2. 提交并打 tag：`git tag v1.0.1 && git push origin v1.0.1`
3. 等 Actions 产出 `PDF2Image-v1.0.1.zip`
4. 用 `tools/发布新版本.py` 生成 `latest.json`，和 zip 一起放到更新目录
   （当前为 `https://www.topbei.cn/pdf2img/`）

## 更新机制

`latest.json`：

```json
{
  "version": "1.0.1",
  "notes": "更新说明",
  "package": "PDF2Image-v1.0.1.zip",
  "package_size": 12345678,
  "package_sha256": "…",
  "exe_name": "PDF2Image.exe"
}
```

程序下载后校验大小与 sha256，解压到临时目录，由新程序等旧进程退出再
整目录覆盖，最后重新启动自己。用户自己的配置（`%APPDATA%\PDF2Image`）
不受影响。
