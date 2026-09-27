#!/usr/bin/env python3
"""Extract vim's and neovim's translated showmode strings from the message
catalogues shipped in the fixture images, so the vim specs can be tested
against every language the editors ship, not a hand-picked few.

usage: vim-markers.py [image-tag...] > tests/fixtures/vim/markers.json
       vim-markers.py --regex < tests/fixtures/vim/markers.json

The second form prints the alternations used in specs/groups/vim-family.toml
(paste them in when the catalogues change; TestVimMarkers checks the spec
against markers.json either way) and fails on a translation that would be
read as the wrong mode.

Each catalogue is decoded with the charset its header declares: vim and
neovim ask gettext to convert to 'encoding' (UTF-8), so that is what ends up
on the screen. vim ships some languages in several encodings (ja,
ja.sjis, ja.euc-jp); decoded, they agree, and are merged under the base name.
"""
import io
import json
import re
import struct
import subprocess
import sys
import tarfile

# The strings showmode() composes as "-- " + ... + " --" (vim screen.c /
# nvim drawscreen.c), plus the more-prompt.
MSGIDS = [
    " INSERT", " REPLACE", " VREPLACE", " TERMINAL",
    " VISUAL", " VISUAL LINE", " VISUAL BLOCK",
    " SELECT", " SELECT LINE", " SELECT BLOCK",
    " (insert)", " (replace)", " (vreplace)",
    " Keyword completion (^N^P)", " Whole line completion (^L^N^P)",
    "-- More --",
    # The ruler's position word, and the hit-enter prompt.
    "All", "Top", "Bot", "Press ENTER or type command to continue",
]


def parse_mo(data):
    magic = struct.unpack("<I", data[:4])[0]
    end = "<" if magic == 0x950412DE else ">"
    _, n, orig, trans = struct.unpack(end + "4I", data[4:20])
    raw = {}
    for i in range(n):
        ol, oo = struct.unpack(end + "2I", data[orig + 8 * i: orig + 8 * i + 8])
        tl, to = struct.unpack(end + "2I", data[trans + 8 * i: trans + 8 * i + 8])
        raw[data[oo: oo + ol]] = data[to: to + tl]
    header = raw.get(b"", b"").decode("ascii", "replace")
    m = re.search(r"charset=([-\w]+)", header)
    charset = m.group(1) if m else "utf-8"
    out = {}
    for mid in MSGIDS:
        v = raw.get(mid.encode())
        if v is not None:
            out[mid] = v.decode(charset, "replace")
    return charset, out


def image_catalogues(tag):
    """(app, version, lang, bytes) for every vim/nvim catalogue in an image."""
    script = r"""
set -e
for v in vim nvim nvim-upstream; do
  command -v $v >/dev/null || continue
  case $v in vim) ver=$(vim --version | head -1 | cut -d' ' -f5) ;;
             *) ver=$($v --version | head -1 | sed 's/^NVIM v//') ;; esac
  echo "$v $ver"
done >/tmp/versions
find / \( -path /proc -o -path /sys \) -prune -o \( -name vim.mo -o -name nvim.mo \) -print |
  tar -c -T - /tmp/versions 2>/dev/null
"""
    tar = subprocess.run(["docker", "run", "--rm", "tmux-modal-fx:" + tag, "sh", "-c", script],
                         check=True, capture_output=True).stdout
    tf = tarfile.open(fileobj=io.BytesIO(tar))
    versions = dict(l.split() for l in tf.extractfile("tmp/versions").read().decode().splitlines())
    for m in tf.getmembers():
        if not m.name.endswith(".mo"):
            continue
        # vim: /usr/share/vim/vimNN/lang/<lang>/LC_MESSAGES/vim.mo
        # nvim (distro): /usr/share/locale/<lang>/LC_MESSAGES/nvim.mo
        # nvim (upstream): /opt/nvim/share/locale/<lang>/LC_MESSAGES/nvim.mo
        lang = m.name.split("/")[-3]
        if m.name.endswith("/vim.mo"):
            app = "vim"
        elif m.name.startswith("opt/nvim/"):
            app = "nvim-upstream"
        else:
            app = "nvim"
        if app in versions:
            yield app, versions[app], lang, tf.extractfile(m).read()


# Mode families: the msgids whose translations one mode rule matches.
FAMILIES = {
    "more": ["-- More --"],
    "hit_enter": ["Press ENTER or type command to continue"],
    "visual": [" VISUAL", " VISUAL LINE", " VISUAL BLOCK"],
    "select": [" SELECT", " SELECT LINE", " SELECT BLOCK"],
    "replace": [" REPLACE", " VREPLACE"],
    "terminal": [" TERMINAL"],
    "ruler": ["All", "Top", "Bot"],
}
# Families whose marker is followed by more words in the same showmode
# string, so a rule for one must not also match another's marker.
# (The "(insert)" pending marker needs no list: "-- (<any>) --".)
MARKERS = ["visual", "select", "replace", "terminal"]
# Words two families share in one language; the earlier rule wins.
AMBIGUOUS = {
    ("select", "選取"),  # zh_TW: visual and select are both 選取; reads as visual
}
TYPING = [" INSERT", " REPLACE", " VREPLACE", " TERMINAL",
          " Keyword completion (^N^P)", " Whole line completion (^L^N^P)"]


def escape(t):
    return "".join("\\" + c if c in "\\.+*?()|[]{}^$" else c for c in t)


def regexes(data):
    words = {f: set() for f in FAMILIES}
    for langs in data.values():
        for strings in list(langs.values()) + [{m: m for ids in FAMILIES.values() for m in ids}]:
            for f, ids in FAMILIES.items():
                for m in ids:
                    if m in strings and (f, strings[m].strip()) not in AMBIGUOUS:
                        words[f].add(strings[m].strip())
    # A rule matches "-- <word>" followed by a space or the end: check no
    # word of one family starts another family's marker that way.
    bad = []
    for langs in data.values():
        for lang, strings in langs.items():
            shown = {m: strings.get(m, m).strip() for ids in FAMILIES.values() for m in ids}
            shown.update({m: strings.get(m, m).strip() for m in TYPING})
            for m, text in shown.items():
                if any((f, text) in AMBIGUOUS for f in FAMILIES if m in FAMILIES[f]):
                    continue
                for f in MARKERS:
                    if m in FAMILIES[f] or (f == "replace" and m in (" REPLACE", " VREPLACE")):
                        continue
                    for w in words[f]:
                        if text == w or text.startswith(w + " "):
                            bad.append(f"{lang}: {m!r} shows {text!r}, which the {f} rule matches ({w!r})")
    out = {}
    for f, ws in words.items():
        out[f] = "|".join(escape(w) for w in sorted(ws, key=lambda w: (-len(w), w)))
    return out, sorted(set(bad))


def main():
    if sys.argv[1:] == ["--regex"]:
        out, bad = regexes(json.load(sys.stdin))
        for f, r in out.items():
            print(f"{f}:\n{r}\n")
        for b in bad:
            print("COLLISION", b, file=sys.stderr)
        sys.exit(1 if bad else 0)
    tags = sys.argv[1:] or ["ubuntu-24.04", "ubuntu-22.04", "debian-bookworm"]
    result = {}
    for tag in tags:
        for app, ver, lang, data in image_catalogues(tag):
            _, strings = parse_mo(data)
            key = ("nvim" if app == "nvim-upstream" else app) + " " + ver
            langs = result.setdefault(key, {})
            base = lang.split(".")[0]
            if base in langs and langs[base] != strings:
                print(f"{key} {lang}: differs from {base}", file=sys.stderr)
                base = lang
            langs.setdefault(base, strings)
    json.dump(result, sys.stdout, ensure_ascii=False, indent=1, sort_keys=True)
    print()


if __name__ == "__main__":
    main()
