#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""用 PaddleOCR 识别本地图片中的文字，把结果以 JSON 写入 --out 指定的文件。

设计要点：
- PaddleOCR 会把大量日志打到 stdout/stderr，直接混入 stdout 会污染结果；
  因此结果一律写文件，Go 侧读文件，彻底避开日志污染。
- 兼容 PaddleOCR 2.x（ocr()）与 3.x（predict()）两套 API。
- 未安装 PaddleOCR 或识别异常时，写 {"error": "..."} 并以非零码退出，
  由 Go 侧转成清晰错误（有容错、无臆造：绝不返回假文字）。
"""
import argparse
import json
import sys


def build_ocr(lang):
    from paddleocr import PaddleOCR

    # 3.x 用 predict() + 新版开关；2.x 用 ocr(cls=True)。逐个尝试构造参数。
    try:
        return PaddleOCR(
            lang=lang,
            use_doc_orientation_classify=False,
            use_doc_unwarping=False,
            use_textline_orientation=False,
        ), "predict"
    except TypeError:
        pass
    try:
        return PaddleOCR(lang=lang, use_angle_cls=True), "ocr"
    except TypeError:
        return PaddleOCR(lang=lang), "ocr"


def run_ocr(ocr, mode, image):
    if mode == "predict":
        return ocr.predict(image)
    try:
        return ocr.ocr(image, cls=True)
    except TypeError:
        return ocr.ocr(image)


def extract_lines(result):
    """把 2.x / 3.x 的返回结构统一抽成文本行列表。"""
    lines = []
    if not result:
        return lines
    first = result[0]
    # 3.x: List[dict]，含 rec_texts / rec_scores
    if isinstance(first, dict):
        for page in result:
            for text in page.get("rec_texts", []) or []:
                t = str(text).strip()
                if t:
                    lines.append(t)
        return lines
    # 2.x: List[List[[box, (text, score)]]]
    for page in result:
        if not isinstance(page, (list, tuple)):
            continue
        for item in page:
            try:
                text = item[1][0]
                t = str(text).strip()
                if t:
                    lines.append(t)
            except (IndexError, TypeError, KeyError):
                continue
    return lines


def write_result(path, payload):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--image", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--lang", default="ch")
    args = ap.parse_args()

    try:
        from paddleocr import PaddleOCR  # noqa: F401
    except ImportError:
        write_result(args.out, {"error": "PaddleOCR 未安装，请先执行：pip install paddleocr paddlepaddle"})
        sys.exit(3)

    try:
        ocr, mode = build_ocr(args.lang)
        result = run_ocr(ocr, mode, args.image)
        lines = extract_lines(result)
        payload = {"lines": lines, "text": "\n".join(lines)}
        write_result(args.out, payload)
        if not lines:
            sys.exit(4)  # 识别成功但无文字
    except Exception as exc:  # noqa: BLE001
        write_result(args.out, {"error": "{}: {}".format(type(exc).__name__, exc)})
        sys.exit(2)


if __name__ == "__main__":
    main()
