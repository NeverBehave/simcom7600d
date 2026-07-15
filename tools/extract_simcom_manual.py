#!/usr/bin/env python3
"""Convert the local SIMCom SIM7500/SIM7600 AT manual to searchable Markdown.

Heuristics:
- Strip header/footer boilerplate.
- Detect chapter headers ("Chapter N ..." or numeric "N. Title").
- Detect numeric command sections ("N.M.K AT+XXX" / "N.M Title").
- Promote those to markdown headings; otherwise emit text.
"""
import argparse
import re
import sys
from pathlib import Path

from pypdf import PdfReader

ROOT = Path(__file__).resolve().parents[1]
REFERENCE_DIR = ROOT / "docs" / "reference" / "simcom"
DEFAULT_PDF = REFERENCE_DIR / "SIM7500_SIM7600_AT_Command_Manual_v3.00.pdf"
DEFAULT_OUT = REFERENCE_DIR / "SIM7500_SIM7600_AT_Command_Manual_v3.00.generated.md"

HEADER_FOOTER_PATTERNS = [
    re.compile(r"^SIM7500_SIM7600 Series_AT Command Manual.*$"),
    re.compile(r"^www\.simcom\.com\s+\d+\s*/\s*\d+\s*$"),
    re.compile(r"^\s*$"),
]

# Section heading patterns
RE_CHAPTER = re.compile(r"^(\d+)\s+(.{2,120})$")  # "1 Introduction"
RE_SUB = re.compile(r"^(\d+\.\d+)\s+(.{2,120})$")  # "1.1 Scope"
RE_CMD = re.compile(r"^(\d+\.\d+\.\d+)\s+(AT[A-Z0-9+&%@#=\-?*]+\s*.*)$")
RE_PARAM_BLOCK = re.compile(r"^(Test Command|Read Command|Write Command|Execution Command|Response|Parameter|Parameters|Reference|Examples?|Note|NOTE)\s*$")


def clean_line(line: str) -> str:
    return line.rstrip()


def is_skip(line: str) -> bool:
    s = line.strip()
    for p in HEADER_FOOTER_PATTERNS:
        if p.match(s):
            return True
    return False


def page_text(reader: PdfReader, i: int) -> list[str]:
    raw = reader.pages[i].extract_text() or ""
    out = []
    for ln in raw.splitlines():
        ln = clean_line(ln)
        if is_skip(ln):
            continue
        out.append(ln)
    return out


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", nargs="?", type=Path, default=DEFAULT_PDF)
    parser.add_argument("output", nargs="?", type=Path, default=DEFAULT_OUT)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if not args.input.is_file():
        raise SystemExit(
            f"Manual not found: {args.input}\n"
            "See docs/reference/simcom/README.md for the expected vendor document."
        )

    r = PdfReader(str(args.input))
    n = len(r.pages)
    sys.stderr.write(f"Pages: {n}\n")

    md_lines: list[str] = []
    md_lines.append("# SIM7500 / SIM7600 Series — AT Command Manual")
    md_lines.append("")
    md_lines.append("> Reference extracted from the SIMCom *SIM7500\\_SIM7600 Series AT Command Manual* v3.00 (2021-11-18).")
    md_lines.append("> Auto-converted from PDF; formatting and line breaks may not perfectly mirror the original.")
    md_lines.append("")

    last_was_blank = True

    def emit(s: str) -> None:
        nonlocal last_was_blank
        md_lines.append(s)
        last_was_blank = (s.strip() == "")

    def blank() -> None:
        nonlocal last_was_blank
        if not last_was_blank:
            md_lines.append("")
            last_was_blank = True

    for i in range(n):
        lines = page_text(r, i)
        for ln in lines:
            stripped = ln.strip()
            if not stripped:
                blank()
                continue

            m_cmd = RE_CMD.match(stripped)
            if m_cmd:
                blank()
                num, title = m_cmd.group(1), m_cmd.group(2).strip()
                emit(f"### {num} {title}")
                blank()
                continue

            m_sub = RE_SUB.match(stripped)
            if m_sub:
                blank()
                num, title = m_sub.group(1), m_sub.group(2).strip()
                emit(f"## {num} {title}")
                blank()
                continue

            m_chap = RE_CHAPTER.match(stripped)
            # Avoid matching e.g. "+CSQ: 20,99" (already starts with +) — RE_CHAPTER requires leading digit only.
            # Avoid promoting tiny numeric values; require letters in title.
            if m_chap and re.search(r"[A-Za-z]", m_chap.group(2)) and len(m_chap.group(2)) >= 4 and not stripped.startswith("+"):
                # Only treat as chapter if number is 1–2 digits and title looks like a heading (capitalised word)
                num = m_chap.group(1)
                title = m_chap.group(2).strip()
                if int(num) < 100 and (title[0].isupper() or title.startswith("AT")):
                    blank()
                    emit(f"# {num}. {title}")
                    blank()
                    continue

            if RE_PARAM_BLOCK.match(stripped):
                blank()
                emit(f"**{stripped}**")
                blank()
                continue

            emit(ln)

    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text("\n".join(md_lines).rstrip() + "\n", encoding="utf-8")
    sys.stderr.write(f"Wrote {args.output}: {len(md_lines)} lines\n")


if __name__ == "__main__":
    main()
