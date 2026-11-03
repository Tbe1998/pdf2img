#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""PDF 转图片助手 —— 原创图标（纯手写几何，无素材、无 AI 生图）。

构图：靛蓝圆角方块 + 一张白纸（折角） + 右下角压着的"照片"卡片。
小尺寸下仍能看出"文档 → 图片"两层意思。
在 2048 画布上绘制，再 LANCZOS 缩到各尺寸。
"""
import os

from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
OUT = os.path.join(HERE, "icons")
S = 2048
SS = 2  # 超采样倍数：先在 2048*2 上画，再缩到 2048


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


def rounded_mask(size, radius):
    m = Image.new("L", (size, size), 0)
    d = ImageDraw.Draw(m)
    d.rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return m


def gradient(size, top, bottom, horizontal=False):
    img = Image.new("RGB", (size, size))
    d = ImageDraw.Draw(img)
    for i in range(size):
        t = i / max(1, size - 1)
        c = lerp(top, bottom, t)
        if horizontal:
            d.line([(i, 0), (i, size)], fill=c)
        else:
            d.line([(0, i), (size, i)], fill=c)
    return img


def build(scale=1):
    W = S * SS
    img = Image.new("RGBA", (W, W), (0, 0, 0, 0))

    # 底：靛蓝渐变圆角方块
    bg = gradient(W, (99, 91, 255), (48, 37, 160)).convert("RGBA")
    bg.putalpha(rounded_mask(W, int(W * 0.225)))
    img.alpha_composite(bg)

    d = ImageDraw.Draw(img)
    k = W / 1024.0  # 以 1024 网格做设计

    # 白纸：主体 + 折角
    px0, py0, px1, py1 = 236 * k, 168 * k, 700 * k, 852 * k
    fold = 118 * k
    paper = [
        (px0, py0 + 24 * k),
        (px1 - fold - 8 * k, py0),
        (px1 - fold, py0),
        (px1, py0 + fold),
        (px1, py1 - 24 * k),
        (px1, py1),
        (px0, py1),
    ]
    d.polygon(paper, fill=(255, 255, 255, 255))
    # 折角三角形（比纸面稍暗）
    d.polygon(
        [(px1 - fold, py0), (px1 - fold, py0 + fold), (px1, py0 + fold)],
        fill=(214, 219, 255, 255),
    )

    # 纸上的文字条（靛蓝淡色）
    lx0, lx1 = 300 * k, 636 * k
    for i, (y, frac) in enumerate(
        [(300, 1.0), (382, 0.92), (464, 0.72), (546, 0.84)]
    ):
        d.rounded_rectangle(
            [lx0, y * k, lx0 + (lx1 - lx0) * frac, y * k + 26 * k],
            radius=13 * k,
            fill=(203, 208, 240, 255) if i else (170, 176, 226, 255),
        )

    # 右下角照片卡片：白边 + 青蓝天空 + 太阳 + 山
    cx0, cy0, cx1, cy1 = 410 * k, 560 * k, 830 * k, 872 * k
    pad = 22 * k
    d.rounded_rectangle(
        [cx0, cy0, cx1, cy1], radius=46 * k, fill=(255, 255, 255, 255)
    )
    sky = gradient(int((cx1 - cx0) - 2 * pad), (86, 196, 235), (37, 122, 190))
    sky = sky.convert("RGBA").resize(
        (int((cx1 - cx0) - 2 * pad), int((cy1 - cy0) - 2 * pad))
    )
    sky_mask = Image.new("L", sky.size, 0)
    ImageDraw.Draw(sky_mask).rounded_rectangle(
        [0, 0, sky.size[0] - 1, sky.size[1] - 1], radius=int(30 * k), fill=255
    )
    img.paste(sky, (int(cx0 + pad), int(cy0 + pad)), sky_mask)

    d = ImageDraw.Draw(img)
    ix0, iy0 = cx0 + pad, cy0 + pad
    ix1, iy1 = cx1 - pad, cy1 - pad
    # 太阳
    d.ellipse(
        [ix0 + 52 * k, iy0 + 46 * k, ix0 + 132 * k, iy0 + 126 * k],
        fill=(255, 214, 102, 255),
    )
    # 远山（浅）
    d.polygon(
        [
            (ix0, iy1 - 96 * k),
            (ix0 + 108 * k, iy1 - 190 * k),
            (ix0 + 196 * k, iy1 - 96 * k),
            (ix0, iy1),
        ],
        fill=(226, 245, 255, 255),
    )
    # 近山（深）
    d.polygon(
        [
            (ix0 + 118 * k, iy1 - 40 * k),
            (ix0 + 236 * k, iy1 - 168 * k),
            (ix0 + 356 * k, iy1 - 40 * k),
            (ix1, iy1 - 108 * k),
            (ix1, iy1),
            (ix0 + 118 * k, iy1),
        ],
        fill=(28, 84, 140, 255),
    )
    return img.resize((S, S), Image.LANCZOS)


def main():
    os.makedirs(OUT, exist_ok=True)
    master = build()
    master.save(os.path.join(OUT, "icon-1024.png"))
    sizes = [512, 256, 128, 64, 48, 32, 16]
    imgs = []
    for s in sizes:
        im = master.resize((s, s), Image.LANCZOS)
        im.save(os.path.join(OUT, "icon-%d.png" % s))
        imgs.append(im)
    # 多尺寸 ico（PIL 用最大图 + sizes 列表）
    master.resize((256, 256), Image.LANCZOS).save(
        os.path.join(HERE, "..", "app.ico"),
        format="ICO",
        sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)],
    )
    print("已生成：", os.path.abspath(os.path.join(HERE, "..", "app.ico")))


if __name__ == "__main__":
    main()
