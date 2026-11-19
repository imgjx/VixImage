# Skills

An Agent turn's `{think, tools[]}` contains the following skills in `tools`. Each tool item must carry `t`; layers may only be written inside `add/set` as `layer`. `view`/`finish` are bottom tools — they must be last and at most one per turn.

| Tool `t` | Purpose | Key fields |
| --- | --- | --- |
| `canvas` | Create/reset canvas | `w`,`h`,`bg`(two colors),`angle`,`solid` |
| `gen` | Generate image → asset (costs steps) | `id`,`p`(English prompt),`size`,`engine`,`ref` |
| `stock` | **Royalty-free stock search** → asset (free) | `id`,`q`(query),`site`,`n` |
| `qrcode` | Generate QR code → asset (free) | `id`,`text`,`size`,`fg`,`bg` |
| `qrdecode` | Decode QR content (read-only) | `src` |
| `ocr` | **PaddleOCR text recognition** (read-only) | `src`,`lang` |
| `htmlrender` | **Render HTML to image** → asset (free) | `id`,`html`,`w`,`h` |
| `add` / `set` | Append/replace layer | `layer` or `layer:[...]`,`i` |
| `del` / `clear` | Delete layer i / clear all | `i` |
| `layers` | List current layers | — |
| `copy` | Copy an asset to a new id | `src`,`as` |
| `undo` | Roll back last n edits | `n`(default 1) |
| `view` | View work (bottom) | `srcs:[asset id/"canvas"/"ref:N"]` |
| `finish` | Finish and label the asset (bottom) | `src` |

## Royalty-Free Stock Library (stock)

The `stock` skill lets the Agent search real royalty-free images, download them as assets, and embed them into posters as `img` layer `src`:

```
{"t":"stock","id":"bg","q":"nebula space","site":"openverse","n":1}
```

- `site`: `openverse` (default, Creative Commons aggregation, **no API key needed**), `pexels`, `pixabay` (the latter two need their own API Key in Settings).
- `n`: 1–8 images; asset ids are `id`, `id_1`, `id_2`, …
- Zero step cost; results (with thumbnails) are fed back to the model for self-check.
- The asset table records source and author so licensing can be verified.
- Openverse requests commercially usable CC images by default; always follow each library's actual license terms.

## OCR Text Recognition (`ocr`)

The `ocr` skill uses **PaddleOCR** to recognize text in an image; the recognized text is fed back **in full** to the model for later layout/verification (e.g. reading text on a reference or generated image):

```
{"t":"ocr","src":"ref:0","lang":"ch"}
```

- `src`: an asset id, `canvas` (rendered current canvas), `ref:N` (Nth input image), or an image URL.
- `lang`: recognition language — defaults to `ch` (Chinese+English), use `en` for English only; a default can also be set in Settings → OCR & HTML Rendering.
- **Read-only**: produces no image asset; the recognized text enters the next-round context.
- Requires PaddleOCR in the Python environment: `pip install paddleocr paddlepaddle`. If not installed, it reports a clear error with the install command — never fabricates text. The Python interpreter path comes from Settings → Python Path.

## HTML Rendering (`htmlrender`)

`htmlrender` renders an HTML snippet into a PNG with headless Chrome and registers it as an asset — ideal for **precise layout of complex text, tables, code, or rich text** (more controllable than drawing each text layer):

```
{"t":"htmlrender","id":"card","html":"<h1 style='color:#fff'>Title</h1>","w":1080,"h":1920}
```

- `html` may be a full HTML document or a fragment; the skill injects a reset stylesheet (zero margins, locked viewport) to avoid white borders / scrollbars.
- `w`/`h`: output size, default 1080×1920; the asset can be used as an `img` layer `src`.
- Zero step cost; the rendered result is fed back as a thumbnail for self-check.
- Requires Chrome/Chromium: when the config path is blank, it auto-detects `google-chrome`/`chromium` etc., or you can set an explicit path in Settings → Chrome Path. Without a browser it reports a clear error — never returns a placeholder image.

## Layer Capabilities (`add` / `set` `layer`)

| Layer `k` | Capabilities |
| --- | --- |
| `img` | `fit` cover/contain, `radius`, `circle`, `shadow`, `border`, overlay `ov`, cutout `cut`, blur, rotate, perspective `persp`(0~0.9)/`shear`(-0.5~0.5), `opacity` |
| `text` | multiline, `anchor`, `spacing`, `italic`, `glow`, `stroke`; `s` uses `\n`, auto-wraps |
| `rect` / `circle` | translucent / circular color blocks with radius and fill |
| `line` / `scrim` | decorative line / gradient scrim (`dir` marks the darkest side) |
| `canvas` | arbitrary W×H (portrait/landscape/square), gradient or solid background |

`src` may reference: a `gen`/`stock`/`htmlrender` asset id, `ref:N` (Nth input image), a `copy` `as`, or an image URL / data URI. Combine `cut`/`blur`/`rot`/`persp`/`shear`/`circle` with `ref:N` to edit a reference image and fit it into any region.

Fonts are auto-detected: prefer `Noto Sans CJK`, compatible with `.otf` and `WenQuanYi`/`PingFang`; Chinese text falls back to a CJK font to avoid tofu boxes.

## Limits

`MaxTextRunes=240` (per text), `MaxLayerCount=80`, `MaxQRText=800`, `MaxHTMLRunes=40000` (per HTML snippet), `MaxUndoSteps=24`, `MaxOutputPerView=6`, `MaxForceReviews=3`; `stock` `n≤8` per call.
