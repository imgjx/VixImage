#!/usr/bin/env python3
"""VixImage 图像编辑技能：纯 PIL 合成/编辑。

读取一个 JSON 规格（--spec），按描述把若干图片与文字绘制到画布上。
支持：
  · 渐变/纯色背景（横向或纵向）
  · 图片放置：cover（裁切填满）/ contain（留白）；圆角；投影；描边
  · scrim 渐变蒙版（用于满幅主视觉上压字）
  · text 文字：多行、锚点、字间距、辉光、描边、阴影
  · line / rect 辅助图形

用法：
    python3 edit_image.py --spec spec.json
spec 例：
{
  "width": 1920, "height": 1080,
  "bg": {"type": "gradient", "colors": ["#0a0e1a", "#101a30"], "angle": 90},
  "items": [
    {"type": "image", "src": "a.png", "x": 0, "y": 0, "w": 1920, "h": 1080,
     "fit": "cover", "radius": 0, "overlay": 0.35},
    {"type": "scrim", "x": 0, "y": 0, "w": 1100, "h": 1080,
     "color": "#04070f", "direction": "right", "strength": 0.92},
    {"type": "text",  "text": "标题文字", "x": 96, "y": 380, "size": 86,
     "color": "#ffffff", "weight": "bold", "glow": "#4da3ff", "glow_size": 26},
    {"type": "image", "src": "b.png", "x": 1400, "y": 760, "w": 440, "h": 260,
     "fit": "cover", "radius": 14, "shadow": true, "border": "#7cc4ff"},
    {"type": "text",  "text": "副标题", "x": 98, "y": 500, "size": 30, "color": "#9ec5ff"}
  ],
  "output": "out.png"
}
"""
import argparse, json, math, os, sys
from PIL import Image, ImageDraw, ImageFilter, ImageFont

FONTS = [
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
    "/usr/share/fonts/truetype/noto/NotoSansCJKsc-Bold.otf",
    "/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
    "/System/Library/Fonts/PingFang.ttc",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
]

MONO_FONTS = [
    "/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf",
    "/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
]


def _pick(paths, weight, mono=False):
    for fp in paths:
        if not os.path.exists(fp):
            continue
        try:
            if fp.endswith(".ttc"):
                return ImageFont.truetype(fp, 10, index=2 if weight == "bold" else 0)
            return ImageFont.truetype(fp, 10)
        except Exception:
            continue
    return ImageFont.load_default()


def load_font(size, weight="regular", mono=False):
    f = _pick(MONO_FONTS if mono else FONTS, weight, mono)
    try:
        if hasattr(f, "size"):
            return f
    except Exception:
        pass
    return f


def _font_at(size, weight, mono):
    """Re-instantiate a truetype font at the requested pixel size."""
    paths = MONO_FONTS if mono else FONTS
    for fp in paths:
        if not os.path.exists(fp):
            continue
        try:
            if fp.endswith(".ttc"):
                idx = 2 if (weight == "bold" or "Bold" in fp) else 0
                return ImageFont.truetype(fp, size, index=idx)
            return ImageFont.truetype(fp, size)
        except Exception:
            continue
    return ImageFont.load_default()


def rounded_mask(w, h, r):
    mask = Image.new("L", (w, h), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, w - 1, h - 1], radius=r, fill=255)
    return mask


def fit_image(src, w, h, mode="cover"):
    """cover = 裁切填满（不留黑边）；contain = 完整显示留白。"""
    im = Image.open(src).convert("RGBA")
    iw, ih = im.size
    if mode == "cover":
        scale = max(w / iw, h / ih)
    else:
        scale = min(w / iw, h / ih)
    nw, nh = max(1, int(iw * scale)), max(1, int(ih * scale))
    im = im.resize((nw, nh), Image.LANCZOS)
    canvas = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    if mode == "cover":
        # 居中裁切，避免出现黑边
        ox = (nw - w) // 2
        oy = (nh - h) // 2
        im = im.crop((ox, oy, ox + w, oy + h))
        canvas.paste(im, (0, 0), im)
    else:
        canvas.paste(im, ((w - nw) // 2, (h - nh) // 2), im)
    return canvas


def hex2rgb(c):
    c = c.lstrip("#")
    if len(c) == 3:
        c = "".join(ch * 2 for ch in c)
    return tuple(int(c[i:i + 2], 16) for i in (0, 2, 4))


def make_bg(spec, w, h):
    bg = spec.get("bg", {})
    if bg.get("type") == "gradient":
        cols = bg.get("colors", ["#0a0e1a", "#101a30"])
        c0 = hex2rgb(cols[0])
        c1 = hex2rgb(cols[-1])
        angle = bg.get("angle", 0)  # 0=横向, 90=纵向
        if angle == 90:
            grad = Image.new("RGB", (1, h), c0)
            px = grad.load()
            for y in range(h):
                t = y / max(1, h - 1)
                px[0, y] = tuple(int(c0[i] + (c1[i] - c0[i]) * t) for i in range(3))
            return grad.resize((w, h))
        grad = Image.new("RGB", (w, 1), c0)
        px = grad.load()
        for x in range(w):
            t = x / max(1, w - 1)
            px[x, 0] = tuple(int(c0[i] + (c1[i] - c0[i]) * t) for i in range(3))
        return grad.resize((w, h))
    color = bg.get("color", "#0a0e1a")
    return Image.new("RGB", (w, h), hex2rgb(color))


def make_scrim(it, w, h):
    """渐变蒙版：direction right/left/bottom/top 指定「最暗」的一侧，alpha 从该侧向外平滑衰减。"""
    sw, sh = it.get("w", w), it.get("h", h)
    col = hex2rgb(it.get("color", "#000000"))
    strength = float(it.get("strength", 0.85))
    direction = it.get("direction", "right")
    layer = Image.new("RGBA", (sw, sh), (0, 0, 0, 0))
    px = layer.load()

    def smooth(t):
        t = max(0.0, min(1.0, t))
        return t * t * (3 - 2 * t)

    for y in range(sh):
        for x in range(sw):
            if direction in ("right", "left"):
                t = x / max(1, sw - 1)
                if direction == "right":      # 左侧最暗，向右衰减
                    f = 1 - smooth(t)
                else:                        # 右侧最暗
                    f = smooth(t)
            else:
                t = y / max(1, sh - 1)
                if direction == "bottom":     # 底部最暗，向上衰减
                    f = smooth(t)
                else:                        # 顶部最暗
                    f = 1 - smooth(t)
            px[x, y] = col + (int(255 * strength * f),)
    return layer


def _spaced_render(draw, lines, x, y, lh, fnt, fill, spacing, anchor):
    """带字间距的逐字绘制：先算整行宽度，再按锚点整体对齐。"""
    widths = []
    for ln in lines:
        cw = sum(draw.textlength(ch, font=fnt) for ch in ln)
        widths.append(cw + spacing * max(0, len(ln) - 1))
    for i, ln in enumerate(lines):
        total = widths[i]
        cx = x
        if anchor[0] == "m":
            cx = x - total / 2
        elif anchor[0] == "r":
            cx = x - total
        for j, ch in enumerate(ln):
            draw.text((cx, y + i * lh), ch, font=fnt, fill=fill, anchor="l" + anchor[1])
            cx += draw.textlength(ch, font=fnt)
            if j < len(ln) - 1:
                cx += spacing


def draw_text(draw, it, w):
    size = int(it.get("size", 30))
    color = it.get("color", "#ffffff")
    weight = it.get("weight", "regular")
    anchor = it.get("anchor", "la")
    mono = it.get("mono", False)
    text = it.get("text", "")
    lines = text.split("\n")
    x, y = it.get("x", 0), it.get("y", 0)
    lh = int(size * float(it.get("line_height", 1.28)))
    spacing = it.get("spacing", 0)
    fnt = _font_at(size, weight, mono)
    if spacing:
        _spaced_render(draw, lines, x, y, lh, fnt, color, spacing, anchor)
    else:
        for i, ln in enumerate(lines):
            draw.text((x, y + i * lh), ln, font=fnt, fill=color, anchor=anchor)
    return y + len(lines) * lh


def draw_text_fx(canvas, it, w):
    """带辉光/描边/阴影的文本（需要模糊，故单独在 canvas 上做）。"""
    size = int(it.get("size", 30))
    weight = it.get("weight", "regular")
    mono = it.get("mono", False)
    anchor = it.get("anchor", "la")
    text = it.get("text", "")
    lines = text.split("\n")
    x, y = it.get("x", 0), it.get("y", 0)
    lh = int(size * float(it.get("line_height", 1.28)))
    spacing = it.get("spacing", 0)
    fnt = _font_at(size, weight, mono)

    def render(target_draw, fill):
        if spacing:
            _spaced_render(target_draw, lines, x, y, lh, fnt, fill, spacing, anchor)
        else:
            for i, ln in enumerate(lines):
                target_draw.text((x, y + i * lh), ln, font=fnt, fill=fill, anchor=anchor)

    # 阴影
    if it.get("shadow", False):
        sl = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
        render(ImageDraw.Draw(sl), (0, 0, 0, 190))
        sl = sl.filter(ImageFilter.GaussianBlur(float(it.get("shadow_blur", 14))))
        canvas.alpha_composite(sl)

    # 辉光
    glow_col = it.get("glow")
    if glow_col:
        gs = int(it.get("glow_size", 24))
        gl = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
        render(ImageDraw.Draw(gl), hex2rgb(glow_col) + (170,))
        gl = gl.filter(ImageFilter.GaussianBlur(gs))
        canvas.alpha_composite(gl)

    # 描边
    stroke = it.get("stroke")
    if stroke:
        sw = int(it.get("stroke_width", 3))
        for dx, dy in [(-sw, 0), (sw, 0), (0, -sw), (0, sw), (-sw, -sw), (sw, -sw), (-sw, sw), (sw, sw)]:
            sl = Image.new("RGBA", canvas.size, (0, 0, 0, 0))
            render(ImageDraw.Draw(sl), hex2rgb(stroke) + (235,))
            canvas.alpha_composite(ImageChopsOffset(sl, dx, dy))
        del ImageChopsOffset

    render(ImageDraw.Draw(canvas), hex2rgb(it.get("color", "#ffffff")) + (255,))


def ImageChopsOffset(layer, dx, dy):
    return layer.transform(layer.size, Image.AFFINE, (1, 0, -dx, 0, 1, -dy),
                           resample=Image.BILINEAR)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--spec", required=True)
    args = ap.parse_args()
    spec = json.load(open(args.spec, encoding="utf-8"))
    w = int(spec.get("width", 1600))
    h = int(spec.get("height", 900))
    canvas = make_bg(spec, w, h).convert("RGBA")
    for it in spec.get("items", []):
        t = it.get("type")
        if t == "image":
            src = it.get("src")
            if not src or not os.path.exists(src):
                continue
            iw, ih = int(it.get("w", 400)), int(it.get("h", 300))
            layer = fit_image(src, iw, ih, it.get("fit", "cover"))
            x, y = int(it.get("x", 0)), int(it.get("y", 0))
            r = int(it.get("radius", 0))
            if r and r > 0:
                layer.putalpha(rounded_mask(iw, ih, r))
            if it.get("shadow", False):
                sh = layer.filter(ImageFilter.GaussianBlur(int(it.get("shadow_blur", 22)))).point(lambda p: p * 0.5)
                canvas.alpha_composite(sh, (x + int(it.get("shadow_dx", 10)), y + int(it.get("shadow_dy", 16))))
            ov = it.get("overlay")
            if ov:
                tint = Image.new("RGBA", (iw, ih), hex2rgb(it.get("overlay_color", "#000000")) + (int(float(ov) * 255),))
                layer = Image.alpha_composite(layer, tint)
            canvas.alpha_composite(layer, (x, y))
            border = it.get("border")
            if border:
                ImageDraw.Draw(canvas).rounded_rectangle(
                    [x, y, x + iw - 1, y + ih - 1],
                    radius=r, outline=hex2rgb(border) + (int(it.get("border_width", 2) * 255 // 2),),
                    width=int(it.get("border_width", 2)))
        elif t == "scrim":
            x, y = int(it.get("x", 0)), int(it.get("y", 0))
            canvas.alpha_composite(make_scrim(it, w, h), (x, y))
        elif t == "text":
            if it.get("glow") or it.get("stroke") or it.get("shadow"):
                draw_text_fx(canvas, it, w)
            else:
                draw_text(ImageDraw.Draw(canvas), it, w)
        elif t == "rect":
            d = ImageDraw.Draw(canvas)
            x, y, rw, rh = it.get("x", 0), it.get("y", 0), it.get("w", 100), it.get("h", 100)
            col = it.get("color", "#ffffff")
            rad = it.get("radius", 0)
            if it.get("fill"):
                d.rounded_rectangle([x, y, x + rw, y + rh], radius=rad,
                                    fill=hex2rgb(col) + (int(it.get("alpha", 1) * 255),))
            d.rounded_rectangle([x, y, x + rw, y + rh], radius=rad,
                                outline=hex2rgb(col) + (int(it.get("alpha", 1) * 255),),
                                width=int(it.get("width", 2)))
        elif t == "line":
            d = ImageDraw.Draw(canvas)
            d.line([tuple(it.get("p1", [0, 0])), tuple(it.get("p2", [0, 0]))],
                   fill=hex2rgb(it.get("color", "#ffffff")) + (int(it.get("alpha", 1) * 255),),
                   width=int(it.get("width", 2)))
    out = spec.get("output", "out.png")
    os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
    canvas.convert("RGB").save(out)
    print("SAVED", out, canvas.size)


if __name__ == "__main__":
    main()
