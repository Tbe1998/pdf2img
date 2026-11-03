@echo off
chcp 65001 >nul
rem 本机重新编译（需要 Go 1.22+，https://go.dev/dl/）
rem 发布包请用仓库根目录 tools\发布新版本.py 生成，或用 GitHub Actions 的产物。
setlocal
cd /d "%~dp0"
set GOOS=windows
set GOARCH=amd64
set CGO_ENABLED=0
go build -trimpath -ldflags "-H windowsgui -s -w" -o "PDF2Image.exe" .
if errorlevel 1 (
  echo 打包失败。
  pause
  exit /b 1
)
echo 已生成 PDF2Image.exe
echo 运行前请把 poppler 的 pdftoppm.exe / pdfinfo.exe 及其 DLL 放进 .\bin 目录。
pause
