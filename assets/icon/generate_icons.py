#!/usr/bin/env python3
"""Generate all multi-platform icon assets for Screen Time.

Reads the clean white-backed master emblems (`source-emblem.jpg` and
`source-small.jpg`), extracts halo-free transparent RGBA masters, and renders
platform-specific variants for Windows, Android, Web/PWA, iOS, and Push badges.
"""

from __future__ import annotations

import io
import math
import os
import struct
from collections import deque
from pathlib import Path
from typing import Dict

from PIL import Image, ImageChops, ImageDraw, ImageFilter, ImageFont

ICON_DIR = Path(__file__).resolve().parent
REPO_ROOT = ICON_DIR.parent.parent

# Palette sampled from concept artwork
GRAD_TL = (118, 198, 253)  # #76C6FD
GRAD_TR = (252, 208, 138)  # #FCD08A
GRAD_BL = (108, 146, 248)  # #6C92F8
GRAD_BR = (255, 153, 104)  # #FF9968

DARK_SLATE = (15, 23, 42, 255)
LIGHT_SLATE = (248, 250, 252, 255)

MONOCHROME_SVG = """<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="96" height="96">
  <path fill="#FFFFFF" fill-rule="evenodd"
        d="M12,2.1C15.9,2.1 19.2,3.3 20.6,4.3L20.6,11.0C20.6,16.5 16.9,20.4 12,22.0C7.1,20.4 3.4,16.5 3.4,11.0L3.4,4.3C4.8,3.3 8.1,2.1 12,2.1ZM12,3.9C8.8,3.9 6.1,4.8 5.1,5.5L5.1,11.0C5.1,15.3 8.0,18.5 12,19.9C16.0,18.5 18.9,15.3 18.9,11.0L18.9,5.5C17.9,4.8 15.2,3.9 12,3.9Z"/>
  <path fill="#FFFFFF"
        d="M5.6,14.5C6.8,17.4 9.3,19.1 12.2,19.2C15.2,19.3 17.8,17.2 18.6,13.6C18.8,12.7 18.0,12.1 17.4,12.7C16.2,14.0 15.7,15.5 14.3,16.7C12.7,17.9 10.1,18.1 8.1,16.9C6.9,16.2 6.0,15.3 5.6,14.5Z"/>
  <path fill="#FFFFFF" fill-rule="evenodd"
        d="M15.9,5.6A2.5,2.5 0 1,1 15.9,10.6A2.5,2.5 0 1,1 15.9,5.6ZM15.4,6.6L16.3,6.6L16.3,8.0L17.4,7.4L17.8,8.2L15.9,9.2L15.4,8.7Z"/>
  <g transform="rotate(-10 11.0 11.8)">
    <path fill="#FFFFFF" fill-rule="evenodd"
          d="M8.4,7.1L13.6,7.1A1.3,1.3 0 0,1 14.9,8.4L14.9,15.2A1.3,1.3 0 0,1 13.6,16.5L8.4,16.5A1.3,1.3 0 0,1 7.1,15.2L7.1,8.4A1.3,1.3 0 0,1 8.4,7.1ZM9.5,9.9A0.85,0.85 0 1,0 9.5,11.6A0.85,0.85 0 1,0 9.5,9.9ZM12.5,9.9A0.85,0.85 0 1,0 12.5,11.6A0.85,0.85 0 1,0 12.5,9.9ZM9.2,12.8C9.9,13.8 12.1,13.8 12.8,12.8L13.5,13.3C12.5,14.7 9.5,14.7 8.5,13.3Z"/>
  </g>
</svg>
"""


def extract_emblem_rgba(src_path: Path) -> Image.Image:
    """Remove exterior white background via 4-corner BFS and decontaminate rim."""
    im = Image.open(src_path).convert("RGB")
    w, h = im.size
    rgb = im.tobytes()
    total = w * h

    def is_bg_pixel(idx: int) -> bool:
        p = idx * 3
        r, g, b = rgb[p], rgb[p + 1], rgb[p + 2]
        return r >= 228 and g >= 228 and b >= 228 and (max(r, g, b) - min(r, g, b)) <= 24

    bg = bytearray(total)
    q: deque[int] = deque()
    for seed in (0, w - 1, (h - 1) * w, total - 1):
        if is_bg_pixel(seed) and not bg[seed]:
            bg[seed] = 1
            q.append(seed)

    while q:
        idx = q.popleft()
        x = idx % w
        y = idx // w
        if x > 0:
            n = idx - 1
            if not bg[n] and is_bg_pixel(n):
                bg[n] = 1
                q.append(n)
        if x + 1 < w:
            n = idx + 1
            if not bg[n] and is_bg_pixel(n):
                bg[n] = 1
                q.append(n)
        if y > 0:
            n = idx - w
            if not bg[n] and is_bg_pixel(n):
                bg[n] = 1
                q.append(n)
        if y + 1 < h:
            n = idx + w
            if not bg[n] and is_bg_pixel(n):
                bg[n] = 1
                q.append(n)

    center_idx = (h // 2) * w + (w // 2)
    assert bg[center_idx] == 0, f"Exterior flood-fill leaked into center of {src_path.name}!"

    # Find 2-pixel external transition ring around bg
    ring = bytearray(total)
    frontier: list[int] = []
    for idx in range(total):
        if bg[idx]:
            continue
        x = idx % w
        y = idx // w
        if (
            (x > 0 and bg[idx - 1])
            or (x + 1 < w and bg[idx + 1])
            or (y > 0 and bg[idx - w])
            or (y + 1 < h and bg[idx + w])
        ):
            ring[idx] = 1
            frontier.append(idx)

    for idx in frontier:
        x = idx % w
        y = idx // w
        for dx, dy in ((-1, 0), (1, 0), (0, -1), (0, 1)):
            nx, ny = x + dx, y + dy
            if 0 <= nx < w and 0 <= ny < h:
                n = ny * w + nx
                if not bg[n] and not ring[n]:
                    ring[n] = 2

    out = bytearray(total * 4)
    for idx in range(total):
        p3 = idx * 3
        p4 = idx * 4
        if bg[idx]:
            out[p4] = 0
            out[p4 + 1] = 0
            out[p4 + 2] = 0
            out[p4 + 3] = 0
            continue

        r, g, b = rgb[p3], rgb[p3 + 1], rgb[p3 + 2]
        if ring[idx]:
            luma = 0.299 * r + 0.587 * g + 0.114 * b
            if luma >= 246.0:
                out[p4] = out[p4 + 1] = out[p4 + 2] = out[p4 + 3] = 0
                continue
            if luma > 58.0:
                alpha_f = max(0.0, min(1.0, (246.0 - luma) / (246.0 - 58.0)))
                a = int(round(alpha_f * 255.0))
                if a <= 4:
                    out[p4] = out[p4 + 1] = out[p4 + 2] = out[p4 + 3] = 0
                    continue
                inv = 1.0 - alpha_f
                r = max(0, min(255, int(round((r - inv * 255.0) / alpha_f))))
                g = max(0, min(255, int(round((g - inv * 255.0) / alpha_f))))
                b = max(0, min(255, int(round((b - inv * 255.0) / alpha_f))))
                out[p4] = r
                out[p4 + 1] = g
                out[p4 + 2] = b
                out[p4 + 3] = a
                continue

        out[p4] = r
        out[p4 + 1] = g
        out[p4 + 2] = b
        out[p4 + 3] = 255

    rgba = Image.frombytes("RGBA", (w, h), bytes(out))
    # Crop strictly to visible pixels (alpha > 8)
    alpha_band = rgba.getchannel("A").point(lambda v: 255 if v > 8 else 0)
    bbox = alpha_band.getbbox()
    assert bbox is not None, f"Empty alpha bbox for {src_path.name}"
    return rgba.crop(bbox)


def place_centered(emblem_tight: Image.Image, canvas_size: int, fill_ratio: float) -> Image.Image:
    """Scale tight emblem proportionally and center on a transparent square canvas."""
    ew, eh = emblem_tight.size
    target_max = max(1, int(round(canvas_size * fill_ratio)))
    scale = target_max / max(ew, eh)
    nw = max(1, int(round(ew * scale)))
    nh = max(1, int(round(eh * scale)))
    resized = emblem_tight.resize((nw, nh), Image.Resampling.LANCZOS)
    canvas = Image.new("RGBA", (canvas_size, canvas_size), (0, 0, 0, 0))
    ox = (canvas_size - nw) // 2
    oy = (canvas_size - nh) // 2
    canvas.alpha_composite(resized, (ox, oy))
    return canvas


def make_gradient_canvas(size: int) -> Image.Image:
    """Render smooth 4-corner bilinear gradient matching concept artwork."""
    base_n = min(size, 256)
    buf = bytearray(base_n * base_n * 3)
    denom = max(1, base_n - 1)
    idx = 0
    for y in range(base_n):
        ty = y / denom
        ity = 1.0 - ty
        for x in range(base_n):
            tx = x / denom
            itx = 1.0 - tx
            w_tl = itx * ity
            w_tr = tx * ity
            w_bl = itx * ty
            w_br = tx * ty
            buf[idx] = int(round(GRAD_TL[0] * w_tl + GRAD_TR[0] * w_tr + GRAD_BL[0] * w_bl + GRAD_BR[0] * w_br))
            buf[idx + 1] = int(round(GRAD_TL[1] * w_tl + GRAD_TR[1] * w_tr + GRAD_BL[1] * w_bl + GRAD_BR[1] * w_br))
            buf[idx + 2] = int(round(GRAD_TL[2] * w_tl + GRAD_TR[2] * w_tr + GRAD_BL[2] * w_bl + GRAD_BR[2] * w_br))
            idx += 3
    img = Image.frombytes("RGB", (base_n, base_n), bytes(buf))
    if base_n != size:
        img = img.resize((size, size), Image.Resampling.BILINEAR)
    return img.convert("RGBA")


def make_squircle_mask(size: int, radius_ratio: float = 0.225, padding_ratio: float = 0.0) -> Image.Image:
    """Create 4x-supersampled continuous rounded-square alpha mask."""
    ss = min(2048, size * 4)
    pad = int(round(ss * padding_ratio))
    inner = ss - 2 * pad
    rad = int(round(inner * radius_ratio))
    mask = Image.new("L", (ss, ss), 0)
    draw = ImageDraw.Draw(mask)
    draw.rounded_rectangle([pad, pad, ss - 1 - pad, ss - 1 - pad], radius=rad, fill=255)
    return mask.resize((size, size), Image.Resampling.LANCZOS)


def make_circle_mask(size: int, padding_ratio: float = 0.04) -> Image.Image:
    """Create 4x-supersampled circular alpha mask."""
    ss = min(2048, size * 4)
    pad = int(round(ss * padding_ratio))
    mask = Image.new("L", (ss, ss), 0)
    draw = ImageDraw.Draw(mask)
    draw.ellipse([pad, pad, ss - 1 - pad, ss - 1 - pad], fill=255)
    return mask.resize((size, size), Image.Resampling.LANCZOS)


def sharpen_rgba(img: Image.Image) -> Image.Image:
    """Apply subtle micro-size crispness to RGB while keeping alpha intact."""
    r, g, b, a = img.split()
    rgb = Image.merge("RGB", (r, g, b)).filter(ImageFilter.UnsharpMask(radius=0.5, percent=25, threshold=5))
    sr, sg, sb = rgb.split()
    return Image.merge("RGBA", (sr, sg, sb, a))


def make_tile(
    emblem_tight: Image.Image,
    size: int,
    fill_ratio: float = 0.80,
    radius_ratio: float = 0.225,
    padding_ratio: float = 0.0,
    sharpen: bool = False,
) -> Image.Image:
    """Composite emblem onto a rounded-square gradient tile."""
    work_size = max(size * 2, 256) if size < 128 else size
    grad = make_gradient_canvas(work_size)
    mask = make_squircle_mask(work_size, radius_ratio=radius_ratio, padding_ratio=padding_ratio)
    tile = grad.copy()
    tile.putalpha(mask)

    effective_fill = (1.0 - 2.0 * padding_ratio) * fill_ratio
    overlay = place_centered(emblem_tight, work_size, effective_fill)
    tile.alpha_composite(overlay)

    if work_size != size:
        tile = tile.resize((size, size), Image.Resampling.LANCZOS)
    if sharpen:
        tile = sharpen_rgba(tile)
    return tile


def make_round_tile(
    emblem_tight: Image.Image,
    size: int,
    fill_ratio: float = 0.72,
    padding_ratio: float = 0.04,
) -> Image.Image:
    """Composite emblem onto a circular gradient tile for legacy round launchers."""
    work_size = max(size * 2, 256)
    grad = make_gradient_canvas(work_size)
    mask = make_circle_mask(work_size, padding_ratio=padding_ratio)
    tile = grad.copy()
    tile.putalpha(mask)
    overlay = place_centered(emblem_tight, work_size, fill_ratio)
    tile.alpha_composite(overlay)
    if work_size != size:
        tile = tile.resize((size, size), Image.Resampling.LANCZOS)
    return tile


def make_fullbleed(emblem_tight: Image.Image, size: int, fill_ratio: float = 0.78) -> Image.Image:
    """Composite emblem onto an opaque full-bleed square gradient (RGB)."""
    grad = make_gradient_canvas(size)
    overlay = place_centered(emblem_tight, size, fill_ratio)
    grad.alpha_composite(overlay)
    return grad.convert("RGB")


def make_monochrome_badge(size: int = 96) -> Image.Image:
    """Render supersampled pure-white + alpha monochrome badge matching SVG."""
    ss = size * 8  # 768x768 for size=96
    s = ss / 24.0

    def pt(x: float, y: float) -> tuple[float, float]:
        return (x * s, y * s)

    def shield_poly(inset: float = 0.0) -> list[tuple[float, float]]:
        # Smooth sampled shield contour matching the vector path
        pts: list[tuple[float, float]] = []
        top_x = [3.5 + i * (17.0 / 16.0) for i in range(17)]
        for x in top_x:
            t = (x - 12.0) / 8.5
            y_top = 2.2 + 2.2 * (t * t)
            pts.append((x, y_top + inset))
        # Right side down
        for i in range(1, 9):
            f = i / 8.0
            y = 4.4 + f * (11.0 - 4.4)
            pts.append((20.5 - inset, y))
        # Bottom right curve
        for i in range(1, 13):
            f = i / 12.0
            ang = f * (math.pi / 2.0)
            x = 12.0 + (8.5 - inset) * math.cos(ang)
            y = 11.0 + (10.9 - inset) * math.sin(ang) ** 0.85
            pts.append((x, y))
        # Bottom left curve
        for i in range(1, 13):
            f = i / 12.0
            ang = math.pi / 2.0 + f * (math.pi / 2.0)
            x = 12.0 + (8.5 - inset) * math.cos(ang)
            y = 11.0 + (10.9 - inset) * math.sin(ang) ** 0.85
            pts.append((x, y))
        # Left side up
        for i in range(1, 9):
            f = i / 8.0
            y = 11.0 - f * (11.0 - 4.4)
            pts.append((3.5 + inset, y))
        return [pt(x, y) for x, y in pts]

    alpha = Image.new("L", (ss, ss), 0)
    draw = ImageDraw.Draw(alpha)

    # 1. Outer shield rim (even-odd outer minus inner)
    draw.polygon(shield_poly(0.0), fill=255)
    draw.polygon(shield_poly(1.8), fill=0)

    # 2. Protective inner cradle curve hugging bottom & right inside shield
    cradle_pts: list[tuple[float, float]] = []
    for i in range(25):
        ang = math.radians(146.0 - i * (146.0 / 24.0))
        x = 11.7 + 5.7 * math.cos(ang)
        y = 11.6 + 5.9 * math.sin(ang)
        cradle_pts.append(pt(x, y))
    for i in range(25):
        ang = math.radians(0.0 + i * (146.0 / 24.0))
        x = 11.8 + 7.4 * math.cos(ang)
        y = 12.0 + 7.5 * math.sin(ang)
        cradle_pts.append(pt(x, y))
    draw.polygon(cradle_pts, fill=255)

    # 3. Upper-right clock peek with negative-space clock hands
    cx, cy, cr = 15.9 * s, 8.1 * s, 2.5 * s
    draw.ellipse([cx - cr, cy - cr, cx + cr, cy + cr], fill=255)
    draw.line([pt(15.9, 6.5), pt(15.9, 8.2), pt(17.5, 7.4)], fill=0, width=int(round(0.85 * s)))

    # 4. Tilted smiling screen character with negative-space eyes & smile
    tablet = Image.new("L", (ss, ss), 0)
    tdraw = ImageDraw.Draw(tablet)
    tdraw.rounded_rectangle([7.0 * s, 7.0 * s, 14.9 * s, 16.5 * s], radius=int(round(1.3 * s)), fill=255)
    # Eyes cutout
    for ex in (9.45, 12.45):
        tdraw.ellipse(
            [(ex - 0.85) * s, (10.6 - 0.85) * s, (ex + 0.85) * s, (10.6 + 0.85) * s],
            fill=0,
        )
    # Smile cutout
    tdraw.arc([8.7 * s, 11.1 * s, 13.2 * s, 14.3 * s], start=20, end=160, fill=0, width=int(round(0.9 * s)))
    tablet_rot = tablet.rotate(10.0, resample=Image.Resampling.BICUBIC, center=(10.95 * s, 11.8 * s))
    alpha = ImageChops.lighter(alpha, tablet_rot)

    alpha_small = alpha.resize((size, size), Image.Resampling.LANCZOS)
    white = Image.new("L", (size, size), 255)
    badge = Image.merge("RGBA", (white, white, white, alpha_small))

    # Guardrail check: every visible pixel must be pure white (255, 255, 255)
    bpx = badge.tobytes()
    for i in range(0, len(bpx), 4):
        if bpx[i + 3] > 0:
            assert bpx[i] == 255 and bpx[i + 1] == 255 and bpx[i + 2] == 255

    return badge


def save_multi_ico(size_to_rgba: Dict[int, Image.Image], path: Path) -> None:
    """Write a canonical Win32 multi-resolution .ico file preserving per-size artwork."""
    ordered_sizes = sorted(size_to_rgba.keys())
    entries: list[tuple[int, int, bytes]] = []

    for sz in ordered_sizes:
        img = size_to_rgba[sz].convert("RGBA")
        assert img.size == (sz, sz)
        if sz >= 256:
            buf = io.BytesIO()
            img.save(buf, format="PNG")
            payload = buf.getvalue()
        else:
            # 32bpp bottom-up BGRA DIB + 1bpp AND mask
            header = struct.pack("<IIIHHIIIIII", 40, sz, sz * 2, 1, 32, 0, sz * sz * 4, 0, 0, 0, 0)
            raw = img.tobytes()
            xor_rows = bytearray()
            and_stride = ((sz + 31) // 32) * 4
            and_rows = bytearray()
            for y in range(sz - 1, -1, -1):
                row_start = y * sz * 4
                and_row = bytearray(and_stride)
                for x in range(sz):
                    p = row_start + x * 4
                    r, g, b, a = raw[p], raw[p + 1], raw[p + 2], raw[p + 3]
                    xor_rows.extend((b, g, r, a))
                    if a == 0:
                        and_row[x // 8] |= 1 << (7 - (x % 8))
                and_rows.extend(and_row)
            payload = header + bytes(xor_rows) + bytes(and_rows)
        entries.append((sz, sz, payload))

    header = struct.pack("<HHH", 0, 1, len(entries))
    offset = 6 + 16 * len(entries)
    dir_bytes = bytearray()
    data_bytes = bytearray()

    for w, h, payload in entries:
        wb = 0 if w >= 256 else w
        hb = 0 if h >= 256 else h
        dir_bytes.extend(struct.pack("<BBBBHHII", wb, hb, 0, 0, 1, 32, len(payload), offset))
        data_bytes.extend(payload)
        offset += len(payload)

    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(header + bytes(dir_bytes) + bytes(data_bytes))


def build_contact_sheet(
    emblem_tile_256: Image.Image,
    small_32: Image.Image,
    small_16: Image.Image,
    adaptive_fg: Image.Image,
    maskable_192: Image.Image,
    apple_180: Image.Image,
    badge_96: Image.Image,
    out_path: Path,
) -> None:
    """Create visual verification sheet across light, dark, masked, and tiny previews."""
    sheet = Image.new("RGBA", (1400, 900), (241, 245, 249, 255))
    draw = ImageDraw.Draw(sheet)
    font = ImageFont.load_default()

    # Panels
    # Top-left: Dark Slate (#0F172A) panel for Windows & halo check
    draw.rounded_rectangle([24, 24, 684, 434], radius=18, fill=DARK_SLATE)
    draw.text((44, 40), "1. DARK SLATE (#0F172A) - TILE, HALO & TINY SIZE CHECK", fill=(148, 163, 184, 255), font=font)
    sheet.alpha_composite(emblem_tile_256, (48, 76))
    draw.text((48, 344), "256px Windows / Web", fill=(226, 232, 240, 255), font=font)

    t48 = emblem_tile_256.resize((48, 48), Image.Resampling.LANCZOS)
    sheet.alpha_composite(t48, (348, 92))
    draw.text((348, 148), "48px", fill=(226, 232, 240, 255), font=font)

    sheet.alpha_composite(small_32, (428, 108))
    draw.text((424, 148), "32px", fill=(226, 232, 240, 255), font=font)

    sheet.alpha_composite(small_16, (492, 124))
    draw.text((484, 148), "16px", fill=(226, 232, 240, 255), font=font)

    # 4x Nearest-neighbor zoom of 32px & 16px simplified tiles
    z32 = small_32.resize((128, 128), Image.Resampling.NEAREST)
    z16 = small_16.resize((64, 64), Image.Resampling.NEAREST)
    sheet.alpha_composite(z32, (348, 200))
    draw.text((348, 336), "32px x4 zoom", fill=(148, 163, 184, 255), font=font)
    sheet.alpha_composite(z16, (512, 232))
    draw.text((506, 304), "16px x4 zoom", fill=(148, 163, 184, 255), font=font)

    # Top-right: Light Slate (#F8FAFC) panel
    draw.rounded_rectangle([716, 24, 1376, 434], radius=18, fill=LIGHT_SLATE, outline=(203, 213, 225, 255), width=2)
    draw.text((736, 40), "2. LIGHT SURFACE (#F8FAFC) - TILE & MICRO LEGIBILITY", fill=(71, 85, 105, 255), font=font)
    sheet.alpha_composite(emblem_tile_256, (740, 76))
    draw.text((740, 344), "256px App Tile", fill=(30, 41, 59, 255), font=font)
    sheet.alpha_composite(t48, (1040, 92))
    sheet.alpha_composite(small_32, (1120, 108))
    sheet.alpha_composite(small_16, (1184, 124))
    sheet.alpha_composite(z32, (1040, 200))
    sheet.alpha_composite(z16, (1204, 232))

    # Bottom-left: Android Adaptive Masks (Circle, Squircle, Rounded, Teardrop)
    draw.rounded_rectangle([24, 460, 740, 876], radius=18, fill=(255, 255, 255, 255), outline=(203, 213, 225, 255), width=2)
    draw.text((44, 476), "3. ANDROID ADAPTIVE LAUNCHER MASKS (58% FG INSIDE 66dp SAFE ZONE)", fill=(71, 85, 105, 255), font=font)

    ada_sz = 144
    ada_bg = make_gradient_canvas(ada_sz)
    ada_fg = adaptive_fg.resize((ada_sz, ada_sz), Image.Resampling.LANCZOS)
    ada_full = ada_bg.copy()
    ada_full.alpha_composite(ada_fg)

    # Mask shapes at 4x supersample
    def apply_mask_shape(shape_name: str) -> Image.Image:
        ss = ada_sz * 4
        m = Image.new("L", (ss, ss), 0)
        md = ImageDraw.Draw(m)
        if shape_name == "Circle":
            md.ellipse([0, 0, ss - 1, ss - 1], fill=255)
        elif shape_name == "Rounded":
            md.rounded_rectangle([0, 0, ss - 1, ss - 1], radius=int(ss * 0.22), fill=255)
        elif shape_name == "Squircle":
            md.rounded_rectangle([0, 0, ss - 1, ss - 1], radius=int(ss * 0.36), fill=255)
        elif shape_name == "Teardrop":
            md.ellipse([0, 0, ss - 1, ss - 1], fill=255)
            md.rectangle([ss // 2, ss // 2, ss - 1, ss - 1], fill=255)
        out_img = ada_full.copy()
        out_img.putalpha(m.resize((ada_sz, ada_sz), Image.Resampling.LANCZOS))
        return out_img

    for i, label in enumerate(("Circle", "Squircle", "Rounded", "Teardrop")):
        x0 = 48 + i * 168
        sheet.alpha_composite(apply_mask_shape(label), (x0, 535))
        draw.text((x0 + 44, 694), label, fill=(30, 41, 59, 255), font=font)

    # Show safe zone guide below
    draw.text((48, 740), "All adaptive foreground artwork fits cleanly inside 61.1% safe zone.", fill=(100, 116, 139, 255), font=font)

    # Bottom-right: PWA Maskable + iOS + Android Notification Bar
    draw.rounded_rectangle([764, 460, 1376, 876], radius=18, fill=(255, 255, 255, 255), outline=(203, 213, 225, 255), width=2)
    draw.text((784, 476), "4. WEB MASKABLE, iOS APPLE-TOUCH & MONOCHROME STATUS BADGE", fill=(71, 85, 105, 255), font=font)

    # Maskable with 80% safe circle guide
    m_prev = maskable_192.convert("RGBA").resize((160, 160), Image.Resampling.LANCZOS)
    sheet.alpha_composite(m_prev, (790, 515))
    # Draw dashed/thin safe circle at 80% diameter (radius = 64 on 160x160)
    cx, cy, r = 790 + 80, 515 + 80, 64
    draw.ellipse([cx - r, cy - r, cx + r, cy + r], outline=(255, 255, 255, 210), width=2)
    draw.text((790, 684), "PWA Maskable (80% safe circle)", fill=(30, 41, 59, 255), font=font)

    # iOS 180px opaque
    ios_prev = apple_180.convert("RGBA").resize((140, 140), Image.Resampling.LANCZOS)
    sheet.alpha_composite(ios_prev, (1010, 525))
    draw.text((1010, 684), "iOS 180px Full-Bleed", fill=(30, 41, 59, 255), font=font)

    # Monochrome badge 96px + 24px simulated Android status bar
    draw.rounded_rectangle([1200, 525, 1344, 669], radius=14, fill=(15, 23, 42, 255))
    sheet.alpha_composite(badge_96, (1224, 549))
    draw.text((1204, 684), "96px Push Badge", fill=(30, 41, 59, 255), font=font)

    # Simulated Android status bar strip
    draw.rounded_rectangle([790, 730, 1344, 790], radius=10, fill=(15, 23, 42, 255))
    b24 = badge_96.resize((24, 24), Image.Resampling.LANCZOS)
    sheet.alpha_composite(b24, (812, 748))
    draw.text((848, 754), "Screen Time Active - Device is monitored and protected", fill=(241, 245, 249, 255), font=font)
    draw.text((790, 804), "Android Status Bar (24dp) & Web Push Notification Badge Preview", fill=(100, 116, 139, 255), font=font)

    out_path.parent.mkdir(parents=True, exist_ok=True)
    sheet.convert("RGB").save(out_path, format="PNG")


def main() -> None:
    emblem_tight = extract_emblem_rgba(ICON_DIR / "source-emblem.jpg")
    small_tight = extract_emblem_rgba(ICON_DIR / "source-small.jpg")

    # 1. Master transparent RGBA assets (1024x1024)
    master_emblem = place_centered(emblem_tight, 1024, 0.84)
    master_small = place_centered(small_tight, 1024, 0.86)
    assert master_emblem.getpixel((512, 512))[3] == 255
    assert master_small.getpixel((512, 512))[3] == 255
    master_emblem.save(ICON_DIR / "master-emblem-1024.png", format="PNG")
    master_small.save(ICON_DIR / "master-small-1024.png", format="PNG")

    # 2. Full-bleed & Squircle tile masters
    master_fullbleed = make_fullbleed(emblem_tight, 1024, fill_ratio=0.78)
    master_fullbleed.save(ICON_DIR / "master-fullbleed-1024.png", format="PNG")

    master_tile = make_tile(emblem_tight, 1024, fill_ratio=0.80, radius_ratio=0.225)
    master_tile.save(ICON_DIR / "master-tile-1024.png", format="PNG")

    playstore_512 = make_fullbleed(emblem_tight, 512, fill_ratio=0.78)
    playstore_512.save(ICON_DIR / "playstore-icon-512.png", format="PNG")

    # 3. Monochrome SVG + Web Push badge-96.png
    (ICON_DIR / "monochrome-badge.svg").write_text(MONOCHROME_SVG, encoding="utf-8")
    badge_96 = make_monochrome_badge(96)
    web_icons_dir = REPO_ROOT / "web" / "public" / "icons"
    web_icons_dir.mkdir(parents=True, exist_ok=True)
    badge_96.save(web_icons_dir / "badge-96.png", format="PNG")

    # 4. Pre-generate small crisp tiles (16, 20, 24, 32) from simplified master
    small_tiles = {
        sz: make_tile(small_tight, sz, fill_ratio=0.88, radius_ratio=0.20, sharpen=True)
        for sz in (16, 20, 24, 32)
    }

    # 5. Windows assets
    win_dir = REPO_ROOT / "client" / "windows"
    win_assets_dir = win_dir / "Assets"
    win_assets_dir.mkdir(parents=True, exist_ok=True)
    win_ico_frames: Dict[int, Image.Image] = {
        16: small_tiles[16],
        20: small_tiles[20],
        24: small_tiles[24],
        32: small_tiles[32],
        40: master_tile.resize((40, 40), Image.Resampling.LANCZOS),
        48: master_tile.resize((48, 48), Image.Resampling.LANCZOS),
        64: master_tile.resize((64, 64), Image.Resampling.LANCZOS),
        128: master_tile.resize((128, 128), Image.Resampling.LANCZOS),
        256: master_tile.resize((256, 256), Image.Resampling.LANCZOS),
    }
    save_multi_ico(win_ico_frames, win_dir / "app.ico")
    win_ico_frames[256].save(win_assets_dir / "app-icon.png", format="PNG")

    # 6. Web / PWA / iOS assets
    web_pub = REPO_ROOT / "web" / "public"
    save_multi_ico(
        {
            16: small_tiles[16],
            32: small_tiles[32],
            48: win_ico_frames[48],
        },
        web_pub / "favicon.ico",
    )
    small_tiles[16].save(web_pub / "favicon-16x16.png", format="PNG")
    small_tiles[32].save(web_pub / "favicon-32x32.png", format="PNG")

    apple_180 = make_fullbleed(emblem_tight, 180, fill_ratio=0.78)
    assert apple_180.mode == "RGB"
    apple_180.save(web_pub / "apple-touch-icon.png", format="PNG")

    icon_192 = master_tile.resize((192, 192), Image.Resampling.LANCZOS)
    icon_512 = master_tile.resize((512, 512), Image.Resampling.LANCZOS)
    icon_192.save(web_icons_dir / "icon-192.png", format="PNG")
    icon_512.save(web_icons_dir / "icon-512.png", format="PNG")

    maskable_192 = make_fullbleed(emblem_tight, 192, fill_ratio=0.64)
    maskable_512 = make_fullbleed(emblem_tight, 512, fill_ratio=0.64)
    maskable_192.save(web_icons_dir / "icon-maskable-192.png", format="PNG")
    maskable_512.save(web_icons_dir / "icon-maskable-512.png", format="PNG")

    # 7. Android mipmap & drawable-nodpi assets
    android_res = REPO_ROOT / "client" / "android" / "app" / "src" / "main" / "res"
    densities = {
        "mdpi": (108, 48),
        "hdpi": (162, 72),
        "xhdpi": (216, 96),
        "xxhdpi": (324, 144),
        "xxxhdpi": (432, 192),
    }
    adaptive_fg_432 = None
    for density, (fg_sz, legacy_sz) in densities.items():
        mdir = android_res / f"mipmap-{density}"
        mdir.mkdir(parents=True, exist_ok=True)

        fg = place_centered(emblem_tight, fg_sz, 0.58)
        fg.save(mdir / "ic_launcher_foreground.png", format="PNG")
        if density == "xxxhdpi":
            adaptive_fg_432 = fg

        legacy_src = small_tight if legacy_sz <= 48 else emblem_tight
        legacy_fill = 0.84 if legacy_sz <= 48 else 0.80
        legacy_sq = make_tile(
            legacy_src,
            legacy_sz,
            fill_ratio=legacy_fill,
            radius_ratio=0.225,
            padding_ratio=0.04,
            sharpen=(legacy_sz <= 48),
        )
        legacy_sq.save(mdir / "ic_launcher.png", format="PNG")

        legacy_round = make_round_tile(
            legacy_src,
            legacy_sz,
            fill_ratio=0.72,
            padding_ratio=0.04,
        )
        legacy_round.save(mdir / "ic_launcher_round.png", format="PNG")

    nodpi_dir = android_res / "drawable-nodpi"
    nodpi_dir.mkdir(parents=True, exist_ok=True)
    win_ico_frames[256].save(nodpi_dir / "ic_app_logo.png", format="PNG")

    # 8. Visual contact sheet in artifact directory
    artifact_sheet = Path(
        r"C:\Users\dean\.gemini\antigravity\brain\396640ca-2d94-426c-8e7f-121dbde95979\icon_preview_sheet.png"
    )
    assert adaptive_fg_432 is not None
    build_contact_sheet(
        win_ico_frames[256],
        small_tiles[32],
        small_tiles[16],
        adaptive_fg_432,
        maskable_192,
        apple_180,
        badge_96,
        artifact_sheet,
    )
    print("Generated all multi-platform icon assets successfully.")


if __name__ == "__main__":
    main()
