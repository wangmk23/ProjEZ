"""Scan public files for common credentials and local identifying paths.

Run with Python 3: python scripts/check_public.py [--include-dist]
Outputs file names and categories only, never matched secret values.
This heuristic check does not replace review of images or unknown formats.
"""
from pathlib import Path
import argparse
import re
import struct
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
RULES = {
    "local-user-path": re.compile(r"(?i)[a-z]:[\\/]Users[\\/][^\s\\/]+"),
    "messenger-account": re.compile(r"wxid_[a-z0-9_]+", re.I),
    "github-token": re.compile(r"(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})"),
    "private-key": re.compile(r"-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----"),
}

def findings(data):
    result = set()
    for encoding in ("utf-8", "utf-16-le", "utf-16-be"):
        text = data.decode(encoding, errors="ignore")
        for label, pattern in RULES.items():
            if pattern.search(text):
                result.add(label)
    return result

def png_metadata(data):
    if not data.startswith(b"\x89PNG\r\n\x1a\n"):
        return []
    found, pos = [], 8
    while pos + 12 <= len(data):
        size = struct.unpack(">I", data[pos:pos+4])[0]
        kind = data[pos+4:pos+8]
        if kind in (b"tEXt", b"zTXt", b"iTXt", b"eXIf"):
            found.append(kind.decode("ascii"))
        pos += size + 12
    return found

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--include-dist", action="store_true")
    args = parser.parse_args()
    errors, count = [], 0
    for p in ROOT.rglob("*"):
        rel = p.relative_to(ROOT)
        if not p.is_file() or ".git" in rel.parts or "__pycache__" in rel.parts:
            continue
        if rel.parts[0] == "dist" and not args.include_dist:
            continue
        data = p.read_bytes()
        count += 1
        for label in sorted(findings(data)):
            errors.append(f"{rel}: {label}")
        if p.suffix.lower() == ".png":
            for label in png_metadata(data):
                errors.append(f"{rel}: image-metadata-{label}")
    # All historical blobs, not just the working tree. Never scan Git packfiles
    # as opaque binaries because compression hides strings.
    if (ROOT / ".git").is_dir():
        objects = subprocess.run(["git", "rev-list", "--objects", "--all"], cwd=ROOT, capture_output=True, check=True).stdout
        for row in objects.splitlines():
            oid = row.split(b" ", 1)[0].decode("ascii")
            kind = subprocess.run(["git", "cat-file", "-t", oid], cwd=ROOT, capture_output=True, check=True).stdout.strip()
            if kind not in (b"blob", b"commit", b"tag"):
                continue
            data = subprocess.run(["git", "cat-file", "-p", oid], cwd=ROOT, capture_output=True, check=True).stdout
            for label in sorted(findings(data)):
                errors.append(f"git-object-{oid[:12]}: {label}")
    for row in errors:
        print(row)
    print(f"Checked {count} files; {len(errors)} findings.")
    return 1 if errors else 0

if __name__ == "__main__":
    sys.exit(main())
