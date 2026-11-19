# 技能（Skills）

Agent 每轮 `{think, tools[]}` 中的 `tools` 由以下技能组成。工具项必须带 `t` 指定类型，图层只能写在 `add/set` 的 `layer` 里。`view`/`finish` 是链底工具，必须放最后一个、每轮最多一个。

| 工具 `t` | 用途 | 关键字段 |
| --- | --- | --- |
| `canvas` | 建立/重设画布 | `w`,`h`,`bg`(两色),`angle`,`solid` |
| `gen` | 生成图片→产物（消耗步数） | `id`,`p`(英文提示词),`size`,`engine`,`ref` |
| `stock` | **免版权图库搜图**→产物（零步） | `id`,`q`(搜索词),`site`,`n` |
| `qrcode` | 生成二维码→产物（零步） | `id`,`text`,`size`,`fg`,`bg` |
| `qrdecode` | 解析二维码内容（只读） | `src` |
| `ocr` | **PaddleOCR 识别图片文字**（只读） | `src`,`lang` |
| `htmlrender` | **HTML 渲染成图片**→产物（零步） | `id`,`html`,`w`,`h` |
| `add` / `set` | 追加/替换图层 | `layer` 或 `layer:[...]`,`i` |
| `del` / `clear` | 删除第 i 层/清空图层 | `i` |
| `layers` | 查看图层清单 | — |
| `copy` | 复制产物为新 id（存档/分叉） | `src`,`as` |
| `undo` | 回滚最近 n 轮编辑 | `n`(默认1) |
| `view` | 查看作品（链底） | `srcs:[产物id/"canvas"/"ref:N"]` |
| `finish` | 完成并标注产物（链底） | `src` |

## 免版权素材库（stock）

`stock` 技能让 Agent 从免版权图库搜索真实图片，下载后登记为产物，可作为 `img` 图层的 `src` 嵌进海报：

```
{"t":"stock","id":"bg","q":"nebula space","site":"openverse","n":1}
```

- `site` 支持：`openverse`（默认，Creative Commons 聚合，**免密钥**）、`pexels`、`pixabay`（后两者需在「系统配置」填各自 API Key）。
- `n` 张数 1~8；多张产物 id 依次为 `id`、`id_1`、`id_2`……
- 零步消耗；结果会回灌给模型（含缩略图）便于自检。
- 产物表会标注来源与作者，便于核对授权信息。
- Openverse 默认只取可商用的 CC 授权图片；请按各图库实际授权规范使用。

## OCR 文字识别（`ocr`）

`ocr` 技能用 **PaddleOCR** 识别图片里的文字，识别出的文本会**完整回灌给模型**用于后续排版/核对（例如读出参考图或生成图上的文字）:

```
{"t":"ocr","src":"ref:0","lang":"ch"}
```

- `src` 支持：产物 id、`canvas`（当前画布渲染结果）、`ref:N`（第 N 张输入图）、图片 URL。
- `lang` 识别语言：默认 `ch`（中英混合），纯英文传 `en`；也可在「系统配置 → OCR 与 HTML 渲染」设默认语言。
- **只读**：不产生图片产物，识别文本进入下一轮上下文。
- 依赖 Python 环境安装 PaddleOCR：`pip install paddleocr paddlepaddle`。未安装时会明确报错并给出安装命令，绝不臆造文字。Python 解释器路径取「系统配置 → Python 路径」。

## HTML 渲染（`htmlrender`）

`htmlrender` 把一段 HTML 用无头 Chrome 渲染成 PNG 并登记为产物，适合**精确排版复杂文字、表格、代码、富文本**（比逐图层画文字更可控）：

```
{"t":"htmlrender","id":"card","html":"<h1 style='color:#fff'>标题</h1>","w":1080,"h":1920}
```

- `html` 可为完整 HTML 文档或片段；技能会注入 reset 样式（去外边距、锁定视口尺寸），避免白边/滚动条。
- `w`/`h` 输出尺寸，默认 1080×1920；产物可当 `img` 图层的 `src` 嵌入海报。
- 零步消耗；渲染结果回灌缩略图供模型自检。
- 依赖 Chrome/Chromium：留空配置时自动探测 `google-chrome`/`chromium` 等，也可在「系统配置 → Chrome 路径」显式指定。未安装浏览器会明确报错，不返回占位图。

## 图层能力（`add` / `set` 的 `layer`）

| 图层 `k` | 能力 |
| --- | --- |
| `img` | `fit` cover/contain、圆角 `radius`、圆形 `circle`、投影 `shadow`、描边 `border`、压暗 `ov`、抠图 `cut`、虚化 `blur`、旋转 `rot`、透视 `persp`(0~0.9)/`shear`(-0.5~0.5)、透明度 `opacity` |
| `text` | 多行、`anchor`、字间距 `spacing`、斜体 `italic`、辉光 `glow`、描边 `stroke`；`s` 用 `\n` 换行，超长自动折行 |
| `rect` / `circle` | 半透明色块 / 圆形色块、圆角、填充 |
| `line` / `scrim` | 装饰线 / 渐变蒙版（`dir` 指定最暗一侧） |
| `canvas` | 任意宽高（竖屏/横版/方形）、渐变或纯色背景 |

`src` 可引用：`gen`/`stock`/`htmlrender` 的产物 id、`ref:N`（第 N 张输入图）、`copy` 的 `as`、图片 URL / data URI。对 `ref:N` 叠加 `cut`/`blur`/`rot`/`persp`/`shear`/`circle` 即可「先编辑参考图、再透视贴合嵌入」。

字体自动探测：优先 `Noto Sans CJK`，兼容 `.otf` 与 `WenQuanYi`/`PingFang`；含中文时自动回退 CJK 字体避免方块字。

## 上限

`MaxTextRunes=240`（单文本）、`MaxLayerCount=80`、`MaxQRText=800`、`MaxHTMLRunes=40000`（单段 HTML）、`MaxUndoSteps=24`、`MaxOutputPerView=6`、`MaxForceReviews=3`；`stock` 单次 `n≤8`。
