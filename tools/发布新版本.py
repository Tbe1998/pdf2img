#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""发布新版本（整包更新）。

用法：
    python 发布新版本.py 新版本号 [更新说明] [解压后的目录]

默认目录是本脚本上一级（仓库根目录），里面应当有：
    PDF2Image.exe、bin\\（poppler）、使用说明.txt

做的事：
    1. exe + bin\\ + 使用说明.txt 打成 PDF2Image-v<版本>.zip；
    2. 算 sha256 与大小；
    3. 生成 latest.json（version / notes / package / package_size /
       package_sha256 / exe_name）；
    4. 把 zip 与 latest.json 一起放到更新目录即可（地址写死在程序里）。

注意：版本号要同时改 core.go 里的 const appVersion，否则客户端不认。
"""
import hashlib
import json
import os
import sys
import zipfile

EXE_NAME = "PDF2Image.exe"
EXTRA_FILES = ["使用说明.txt", "README.md"]


def sha256_of(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    version = sys.argv[1].lstrip("vV")
    notes = sys.argv[2] if len(sys.argv) > 2 else ""
    here = os.path.dirname(os.path.abspath(__file__))
    root = sys.argv[3] if len(sys.argv) > 3 else os.path.dirname(here)

    exe = os.path.join(root, EXE_NAME)
    bindir = os.path.join(root, "bin")
    if not os.path.isfile(exe):
        print("找不到 exe：%s" % exe)
        return 1
    if not os.path.isdir(bindir):
        print("找不到 bin 目录（poppler）：%s" % bindir)
        return 1
    if not os.path.isfile(os.path.join(bindir, "pdftoppm.exe")):
        print("bin 目录里没有 pdftoppm.exe，安装包不完整")
        return 1

    zip_name = "PDF2Image-v%s.zip" % version
    zip_path = os.path.join(here, zip_name)

    with zipfile.ZipFile(zip_path, "w", zipfile.ZIP_DEFLATED, compresslevel=9) as z:
        z.write(exe, EXE_NAME)
        for name in sorted(os.listdir(bindir)):
            p = os.path.join(bindir, name)
            if os.path.isfile(p):
                z.write(p, "bin/" + name)
        for extra in EXTRA_FILES:
            p = os.path.join(root, extra)
            if os.path.isfile(p):
                z.write(p, extra)

    manifest = {
        "version": version,
        "notes": notes,
        "package": zip_name,
        "package_size": os.path.getsize(zip_path),
        "package_sha256": sha256_of(zip_path),
        "exe_name": EXE_NAME,
    }
    out = os.path.join(here, "latest.json")
    with open(out, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=2)

    print("已生成：", zip_path)
    print("已生成：", out)
    print("包大小：%.1f MB" % (manifest["package_size"] / 1024 / 1024))
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    print("\n下一步：把 %s 和 latest.json 传到更新目录。" % zip_name)
    print("别忘了先把 core.go 里的 appVersion 改成 %s。" % version)
    return 0


if __name__ == "__main__":
    sys.exit(main())
