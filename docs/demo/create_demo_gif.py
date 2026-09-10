#!/usr/bin/env python3
"""Build the OpenSync 60-second demo GIF from existing screenshots.

Usage:
  python3 -m pip install pillow
  python3 docs/demo/create_demo_gif.py
"""

from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont, ImageOps


ROOT = Path(__file__).resolve().parents[2]
IMAGE_DIR = ROOT / "docs" / "images"
OUTPUT = ROOT / "docs" / "demo" / "opensync-demo.gif"

CANVAS = (1280, 720)
BG = "#f7f4ee"
INK = "#17202a"
INK_SOFT = "#46515d"
TEAL = "#0f766e"
BLUE = "#2563eb"
AMBER = "#d98a1b"
LINE = "#d7e0e6"
WHITE = "#ffffff"

FONT_CANDIDATES = [
    "/System/Library/Fonts/Hiragino Sans GB.ttc",
    "/System/Library/Fonts/STHeiti Medium.ttc",
    "/System/Library/Fonts/Supplemental/Songti.ttc",
    "/System/Library/Fonts/SFNS.ttf",
]


def font(size: int) -> ImageFont.FreeTypeFont | ImageFont.ImageFont:
    for candidate in FONT_CANDIDATES:
        try:
            return ImageFont.truetype(candidate, size=size)
        except OSError:
            continue
    return ImageFont.load_default()


FONT_TITLE = font(42)
FONT_SUBTITLE = font(24)
FONT_BODY = font(25)
FONT_SMALL = font(20)
FONT_BADGE = font(18)


def rounded_rect(draw: ImageDraw.ImageDraw, box, radius: int, fill, outline=None, width: int = 1):
    draw.rounded_rectangle(box, radius=radius, fill=fill, outline=outline, width=width)


def fit_image(path: Path, size: tuple[int, int]) -> Image.Image:
    image = Image.open(path).convert("RGB")
    image = ImageOps.contain(image, size, method=Image.Resampling.LANCZOS)
    framed = Image.new("RGB", size, WHITE)
    framed.paste(image, ((size[0] - image.width) // 2, (size[1] - image.height) // 2))
    return framed


def draw_wrapped(draw: ImageDraw.ImageDraw, text: str, xy: tuple[int, int], max_width: int, line_gap: int = 8):
    x, y = xy
    line = ""
    for char in text:
        test = line + char
        if draw.textbbox((x, y), test, font=FONT_BODY)[2] - x <= max_width:
            line = test
            continue
        draw.text((x, y), line, fill=INK_SOFT, font=FONT_BODY)
        y += FONT_BODY.size + line_gap
        line = char
    if line:
        draw.text((x, y), line, fill=INK_SOFT, font=FONT_BODY)
        y += FONT_BODY.size + line_gap
    return y


def add_step_bar(draw: ImageDraw.ImageDraw, active: int, total: int = 6):
    x0, y, gap = 72, 638, 14
    w = (CANVAS[0] - 144 - gap * (total - 1)) // total
    labels = ["引擎", "任务", "运行", "进度", "通知", "复盘"]
    for idx in range(total):
        fill = TEAL if idx + 1 <= active else "#e8edf0"
        rounded_rect(draw, (x0 + idx * (w + gap), y, x0 + idx * (w + gap) + w, y + 10), 5, fill)
        text_fill = INK if idx + 1 == active else "#7c8792"
        draw.text((x0 + idx * (w + gap), y + 18), labels[idx], fill=text_fill, font=FONT_BADGE)


def make_frame(step: int, title: str, subtitle: str, screenshot: str, accent: str, active: int) -> Image.Image:
    canvas = Image.new("RGB", CANVAS, BG)
    draw = ImageDraw.Draw(canvas)

    draw.rectangle((0, 0, CANVAS[0], 96), fill="#fffaf2")
    draw.text((72, 28), "OpenSync", fill=INK, font=FONT_TITLE)
    draw.text((310, 42), "AList / OpenList 自动同步工具", fill=INK_SOFT, font=FONT_SMALL)

    rounded_rect(draw, (72, 128, 438, 580), 18, WHITE, outline=LINE)
    rounded_rect(draw, (96, 154, 170, 188), 17, accent)
    draw.text((112, 158), f"步骤 {step}", fill=WHITE, font=FONT_BADGE)
    draw.text((96, 218), title, fill=INK, font=FONT_TITLE)
    draw_wrapped(draw, subtitle, (98, 286), 300)

    rounded_rect(draw, (96, 502, 210, 540), 19, "#ecfdf5")
    draw.text((116, 510), "Docker", fill=TEAL, font=FONT_BADGE)
    rounded_rect(draw, (222, 502, 350, 540), 19, "#eff6ff")
    draw.text((244, 510), "fnOS / NAS", fill=BLUE, font=FONT_BADGE)

    rounded_rect(draw, (476, 128, 1208, 580), 18, WHITE, outline=LINE)
    shot = fit_image(IMAGE_DIR / screenshot, (684, 404))
    canvas.paste(shot, (500, 152))

    draw.rectangle((500, 526, 1184, 556), fill="#f7fbfc")
    draw.text((520, 532), "真实界面截图 / Real UI screenshot", fill="#7c8792", font=FONT_BADGE)

    add_step_bar(draw, active)
    return canvas


def make_intro() -> Image.Image:
    canvas = Image.new("RGB", CANVAS, BG)
    draw = ImageDraw.Draw(canvas)
    draw.rectangle((0, 0, CANVAS[0], CANVAS[1]), fill="#fffaf2")
    draw.text((92, 118), "OpenSync", fill=INK, font=font(76))
    draw.text((98, 214), "AList / OpenList 自动同步工具", fill=INK_SOFT, font=font(32))
    draw.text((98, 276), "备份、镜像、归档、迁移", fill=INK_SOFT, font=font(32))
    draw.text((98, 330), "一套可视化任务台完成", fill=INK_SOFT, font=font(32))
    rounded_rect(draw, (98, 364, 344, 412), 24, "#ecfdf5")
    draw.text((124, 374), "飞牛 NAS", fill=TEAL, font=FONT_SUBTITLE)
    rounded_rect(draw, (370, 364, 584, 412), 24, "#eff6ff")
    draw.text((398, 374), "Docker", fill=BLUE, font=FONT_SUBTITLE)
    rounded_rect(draw, (98, 436, 344, 484), 24, "#fff7ed")
    draw.text((126, 446), "OpenList", fill=AMBER, font=FONT_SUBTITLE)
    shot = fit_image(IMAGE_DIR / "tasks-overview.png", (492, 292))
    rounded_rect(draw, (690, 166, 1198, 474), 18, WHITE, outline=LINE)
    canvas.paste(shot, (698, 174))
    draw.text((98, 566), "添加引擎 -> 新建任务 -> 手动运行 -> 实时进度 -> 通知", fill=INK, font=FONT_SUBTITLE)
    add_step_bar(draw, 0)
    return canvas


def make_outro() -> Image.Image:
    canvas = Image.new("RGB", CANVAS, BG)
    draw = ImageDraw.Draw(canvas)
    draw.text((86, 110), "适合这些场景", fill=INK, font=font(58))
    items = [
        ("照片库备份", "把 fnOS / NAS 本地目录备份到网盘或对象存储"),
        ("下载目录归档", "下载完成后自动移动到 WebDAV、网盘或另一台设备"),
        ("多存储迁移", "在不同 AList / OpenList 后端之间迁移或镜像文件"),
    ]
    y = 226
    for idx, (name, desc) in enumerate(items, start=1):
        rounded_rect(draw, (92, y, 1110, y + 92), 18, WHITE, outline=LINE)
        rounded_rect(draw, (120, y + 24, 170, y + 74), 25, TEAL if idx == 1 else BLUE if idx == 2 else AMBER)
        draw.text((137, y + 31), str(idx), fill=WHITE, font=FONT_SUBTITLE)
        draw.text((194, y + 18), name, fill=INK, font=FONT_SUBTITLE)
        draw.text((194, y + 54), desc, fill=INK_SOFT, font=FONT_SMALL)
        y += 118
    draw.text((92, 610), "github.com/chenbin3625/OpenSync", fill=TEAL, font=FONT_SUBTITLE)
    add_step_bar(draw, 6)
    return canvas


def main() -> None:
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    frames = [
        make_intro(),
        make_frame(1, "添加引擎", "保存 AList / OpenList 地址与令牌，保存前先验证连接，已保存令牌不会回显。", "engines.png", TEAL, 1),
        make_frame(2, "新建任务", "选择源目录、目标目录和同步模式，把备份、镜像或迁移配置成可重复执行的任务。", "tasks-overview.png", BLUE, 2),
        make_frame(3, "手动运行", "任务卡片上可一键执行、停止、编辑或删除，也支持间隔和 Cron 定时。", "tasks-overview.png", AMBER, 3),
        make_frame(4, "实时进度", "运行时查看扫描数量、传输速度、剩余时间，以及成功、失败、等待和运行中文件。", "realtime-task.png", TEAL, 4),
        make_frame(5, "收到通知", "支持 Webhook、钉钉、企业微信和飞书；无变化时可静默。", "notifications.png", BLUE, 5),
        make_frame(6, "历史复盘", "任务结束后查看详情，定位失败文件，并重试未完成项目。", "task-detail.png", AMBER, 6),
        make_outro(),
    ]
    durations = [5000, 8000, 8000, 7000, 10000, 8000, 7000, 7000]
    frames[0].save(
        OUTPUT,
        save_all=True,
        append_images=frames[1:],
        duration=durations,
        loop=0,
        optimize=True,
    )
    print(f"Wrote {OUTPUT}")


if __name__ == "__main__":
    main()
