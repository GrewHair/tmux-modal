#!/usr/bin/env python3
"""Extract debconf's translated backtitle ("Package configuration", which
the dialog frontend draws on row 0 of every question) from the debconf-i18n
catalogues in the fixture images, so specs/debconf.toml covers every
language debconf ships.

usage: debconf-titles.py [image-tag...] > tests/fixtures/debconf/titles.json
       debconf-titles.py --regex < tests/fixtures/debconf/titles.json

The second form prints the alternation for the backtitle clause in
specs/debconf.toml (paste it in when the catalogues change;
TestDebconfTitles checks the spec against titles.json either way).
"""
import json
import re
import subprocess
import sys

MSGID = "Package configuration"

# Runs inside the image: every debconf.mo, decoded with its own charset.
# Read raw, not with gettext: Python's gettext rejects the Plural-Forms
# headers of some catalogues (bs, he) that GNU gettext accepts.
EXTRACT = r"""
import glob, json, re, struct
out = {}
for mo in sorted(glob.glob("/usr/share/locale/*/LC_MESSAGES/debconf.mo")):
    data = open(mo, "rb").read()
    end = "<" if struct.unpack("<I", data[:4])[0] == 0x950412DE else ">"
    _, n, orig, trans = struct.unpack(end + "4I", data[4:20])
    raw = {}
    for i in range(n):
        ol, oo = struct.unpack(end + "2I", data[orig + 8 * i: orig + 8 * i + 8])
        tl, to = struct.unpack(end + "2I", data[trans + 8 * i: trans + 8 * i + 8])
        raw[data[oo: oo + ol]] = data[to: to + tl]
    m = re.search(rb"charset=([-\w]+)", raw.get(b"", b""))
    s = raw.get(b"%s")
    if s:
        out[mo.split("/")[4]] = s.decode(m.group(1).decode() if m else "utf-8")
print(json.dumps(out, ensure_ascii=False))
""" % MSGID


def image_titles(tag):
    out = subprocess.run(
        ["docker", "run", "--rm", "-i", "--entrypoint", "", "tmux-modal-fx:" + tag, "python3", "-"],
        input=EXTRACT, stdout=subprocess.PIPE, text=True, check=True).stdout
    return json.loads(out)


def main():
    if sys.argv[1:] == ["--regex"]:
        titles = json.load(sys.stdin)
        words = {MSGID} | {t.strip() for per in titles.values() for t in per.values()}
        print("^(" + "|".join(re.escape(w).replace("\\ ", " ") for w in sorted(words)) + ")$")
        return
    tags = sys.argv[1:] or ["ubuntu-24.04", "ubuntu-22.04", "debian-bookworm"]
    by_lang = {}
    for tag in tags:
        for lang, title in image_titles(tag).items():
            by_lang.setdefault(lang, {})[tag] = title
    json.dump(by_lang, sys.stdout, ensure_ascii=False, indent=1, sort_keys=True)
    print()


if __name__ == "__main__":
    main()
