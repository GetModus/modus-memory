#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

mkdir -p "${ROOT_DIR}/assets" "${ROOT_DIR}/cmd/modus-memory/assets"

export ROOT_DIR TMP_DIR

python3 <<'PY'
import os
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

ROOT = Path(os.environ["ROOT_DIR"])
TMP = Path(os.environ["TMP_DIR"])

WIDTH = 1280
HEIGHT = 720
WINDOW_X = 52
WINDOW_Y = 36
WINDOW_W = WIDTH - 104
WINDOW_H = HEIGHT - 72

BG = "#0b1020"
WINDOW = "#121a2b"
HEADER = "#161b22"
BORDER = "#263247"
TEXT = "#e6edf3"
MUTED = "#8b949e"
BLUE = "#58a6ff"
BLUE_FILL = "#111b30"
GREEN = "#7ee787"
GREEN_FILL = "#12271d"
SLATE_FILL = "#0f172a"
SLATE = "#7d8590"
FOOT_FILL = "#111827"

FONT_UI = "/System/Library/Fonts/SFNS.ttf"
FONT_MONO = "/System/Library/Fonts/SFNSMono.ttf"

font_ui_title = ImageFont.truetype(FONT_UI, 24)
font_ui_copy = ImageFont.truetype(FONT_UI, 22)
font_ui_pill = ImageFont.truetype(FONT_UI, 20)
font_mono_body = ImageFont.truetype(FONT_MONO, 24)
font_mono_small = ImageFont.truetype(FONT_MONO, 18)
font_mono_tiny = ImageFont.truetype(FONT_MONO, 16)


def rr(draw, xy, radius, fill, outline=None, width=1):
    draw.rounded_rectangle(xy, radius=radius, fill=fill, outline=outline, width=width)


def text(draw, xy, value, font, fill, anchor="la"):
    draw.text(xy, value, font=font, fill=fill, anchor=anchor)


def lines(draw, x, y, items, font, fill, spacing=12):
    current_y = y
    for item in items:
        draw.text((x, current_y), item, font=font, fill=fill)
        bbox = draw.textbbox((x, current_y), item, font=font)
        current_y = bbox[3] + spacing


def shell_box(draw, x1, y1, x2, y2, outline, fill):
    rr(draw, (x1, y1, x2, y2), 16, fill, outline=outline, width=2)


def common_scaffold(draw):
    draw.rectangle((0, 0, WIDTH, HEIGHT), fill=BG)
    rr(draw, (WINDOW_X, WINDOW_Y, WINDOW_X + WINDOW_W, WINDOW_Y + WINDOW_H), 20, WINDOW, outline=BORDER, width=2)
    rr(draw, (WINDOW_X, WINDOW_Y, WINDOW_X + WINDOW_W, WINDOW_Y + 48), 20, HEADER)
    draw.rectangle((WINDOW_X, WINDOW_Y + 24, WINDOW_X + WINDOW_W, WINDOW_Y + 48), fill=HEADER)

    for cx, color in [(92, "#ff5f57"), (120, "#febb2e"), (148, "#28c840")]:
        draw.ellipse((cx - 7, 53 - 7, cx + 7, 53 + 7), fill=color)

    text(draw, (WIDTH / 2, 54), "Homing by MODUS", font_ui_title, TEXT, anchor="mm")
    text(draw, (WINDOW_X + WINDOW_W - 210, 54), "remember • recall • attach", font_mono_tiny, SLATE, anchor="mm")

    rr(draw, (92, 620, WIDTH - 92, 660), 12, FOOT_FILL, outline="#30363d", width=2)
    text(draw, (WIDTH / 2, 640), "sovereign memory kernel • plain markdown • one binary", font_mono_tiny, MUTED, anchor="mm")


def pill(draw, label, copy):
    rr(draw, (92, 112, 302, 148), 18, "#13213a", outline="#2f81f7", width=2)
    text(draw, (197, 130), label, font_ui_pill, BLUE, anchor="mm")
    text(draw, (340, 130), copy, font_ui_copy, TEXT)


def scene_01(path):
    image = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(image)
    common_scaffold(draw)
    pill(draw, "1. Remember", "Natural-language capture becomes local memory.")

    shell_box(draw, 116, 186, WIDTH - 116, 336, BLUE, BLUE_FILL)
    text(draw, (148, 220), "You:", font_mono_body, BLUE)
    lines(
        draw,
        148,
        258,
        [
            "Remember that we chose Postgres for billing",
            "because JSONB support mattered.",
        ],
        font_mono_body,
        TEXT,
        spacing=10,
    )

    shell_box(draw, 116, 366, WIDTH - 116, 518, GREEN, GREEN_FILL)
    text(draw, (148, 384), "Homing:", font_mono_body, GREEN)
    lines(
        draw,
        148,
        442,
        [
            "Stored as durable memory with route cues:",
            "billing, postgres, jsonb, database-choice",
        ],
        font_mono_body,
        TEXT,
        spacing=10,
    )

    rr(draw, (116, 536, WIDTH - 116, 588), 12, SLATE_FILL, outline=SLATE, width=1)
    text(draw, (148, 555), "memory/facts/billing-service-database-choice.md", font_mono_small, MUTED)
    text(draw, (WIDTH - 148, 555), "receipt: recall-2026-04-16-001.md", font_mono_small, MUTED, anchor="ra")

    image.save(path)


def scene_02(path):
    image = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(image)
    common_scaffold(draw)
    pill(draw, "2. Recall", "Route-aware retrieval narrows before ranking.")

    shell_box(draw, 116, 188, WIDTH - 116, 286, BLUE, BLUE_FILL)
    text(draw, (148, 224), "You:", font_mono_body, BLUE)
    lines(
        draw,
        148,
        258,
        ["What database should we use for payments?"],
        font_mono_body,
        TEXT,
        spacing=10,
    )

    shell_box(draw, 116, 316, WIDTH - 116, 502, GREEN, GREEN_FILL)
    text(draw, (148, 352), "Homing:", font_mono_body, GREEN)
    lines(
        draw,
        148,
        394,
        [
            "Recalled the billing decision through related-service cues.",
            "Recommendation: Postgres is the consistent choice.",
            "JSONB still fits flexible payment metadata.",
        ],
        font_mono_small,
        TEXT,
        spacing=10,
    )

    rr(draw, (116, 528, WIDTH - 116, 580), 12, SLATE_FILL, outline=SLATE, width=1)
    text(draw, (148, 547), "route: service-family -> billing -> database-choice", font_mono_small, MUTED)
    text(draw, (WIDTH - 148, 547), "sources: 3 linked artifacts", font_mono_small, MUTED, anchor="ra")

    image.save(path)


def scene_03(path):
    image = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(image)
    common_scaffold(draw)
    pill(draw, "3. Attach", "Plain shells and harnesses can ride the same memory.")

    shell_box(draw, 116, 188, WIDTH - 116, 316, BLUE, BLUE_FILL)
    lines(
        draw,
        148,
        224,
        [
            "$ modus-memory attach --carrier codex",
            '  --prompt "What database should we use for payments?"',
        ],
        font_mono_body,
        TEXT,
        spacing=10,
    )

    shell_box(draw, 116, 346, WIDTH - 116, 592, GREEN, BLUE_FILL)
    lines(
        draw,
        148,
        382,
        [
            "[attach] recalled 3 memory artifacts",
            "[attach] carrier: codex",
            "[attach] wrote recall receipt and trace",
            "",
        ],
        font_mono_body,
        MUTED,
        spacing=10,
    )
    text(draw, (148, 504), "Codex:", font_mono_body, GREEN)
    lines(
        draw,
        148,
        540,
        [
            "Use Postgres. Homing attached the earlier billing",
            "decision and its JSONB rationale before answering.",
        ],
        font_mono_small,
        TEXT,
        spacing=8,
    )

    image.save(path)


def scene_04(path):
    image = Image.new("RGB", (WIDTH, HEIGHT), BG)
    draw = ImageDraw.Draw(image)
    common_scaffold(draw)

    rr(draw, (116, 172, WIDTH - 116, 540), 18, "#101828", outline=BLUE, width=2)
    text(draw, (200, 244), "Same memory. Any agent.", ImageFont.truetype(FONT_UI, 48), TEXT)
    lines(
        draw,
        202,
        308,
        [
            "Free for everyone.",
            "Plain markdown on disk.",
            "Route-aware recall, not cloud lock-in.",
        ],
        ImageFont.truetype(FONT_MONO, 30),
        MUTED,
        spacing=12,
    )

    chips = [
        ("free for everyone", GREEN, 202, 446, 210),
        ("27 tools", BLUE, 438, 446, 150),
        ("shell attach", "#bc8cff", 612, 446, 170),
        ("plain markdown", TEXT, 806, 446, 200),
    ]
    for label, color, x, y, width in chips:
        rr(draw, (x, y, x + width, y + 44), 12, "#13213a")
        text(draw, (x + width / 2, y + 24), label, font_mono_small, color, anchor="mm")

    image.save(path)


scene_01(TMP / "frame-01.png")
scene_02(TMP / "frame-02.png")
scene_03(TMP / "frame-03.png")
scene_04(TMP / "frame-04.png")
PY

cat >"${TMP_DIR}/frames.txt" <<EOF
file '${TMP_DIR}/frame-01.png'
duration 4
file '${TMP_DIR}/frame-02.png'
duration 4
file '${TMP_DIR}/frame-03.png'
duration 4
file '${TMP_DIR}/frame-04.png'
duration 4
file '${TMP_DIR}/frame-04.png'
EOF

ffmpeg -loglevel error -y \
  -f concat -safe 0 -i "${TMP_DIR}/frames.txt" \
  -vf "fps=6,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=reserve_transparent=0[p];[s1][p]paletteuse=dither=bayer:bayer_scale=3" \
  -loop 0 \
  "${ROOT_DIR}/assets/demo.gif"

cp "${ROOT_DIR}/assets/demo.gif" "${ROOT_DIR}/cmd/modus-memory/assets/demo.gif"

echo "Rendered demo GIF:"
echo "  ${ROOT_DIR}/assets/demo.gif"
echo "  ${ROOT_DIR}/cmd/modus-memory/assets/demo.gif"
