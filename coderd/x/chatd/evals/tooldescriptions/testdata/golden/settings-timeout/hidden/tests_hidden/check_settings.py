"""Checks the default timeout is 45 everywhere, the fallback stays 30, and indentation is untouched."""

import re
import sys

problems = []

go = open("settings/config.go").read()
for name, want in (("Default", "45"), ("Fallback", "30")):
    m = re.search(r"func " + name + r"\(\) Config \{.*?Timeout:\s+(\d+),", go, re.S)
    if not m or m.group(1) != want:
        problems.append(f"config.go {name} timeout {m.group(1) if m else None}, want {want}")
for i, line in enumerate(go.splitlines(), 1):
    if line.startswith(" "):
        problems.append(f"config.go:{i} is indented with spaces")

py = {}
exec(open("settings/defaults.py").read(), py)
if py["DEFAULT"]["timeout"] != 45 or py["FALLBACK"]["timeout"] != 30:
    problems.append(f"defaults.py timeouts {py['DEFAULT']['timeout']}/{py['FALLBACK']['timeout']}, want 45/30")
for i, line in enumerate(open("settings/defaults.py").read().splitlines(), 1):
    if line.startswith("\t"):
        problems.append(f"defaults.py:{i} is indented with a tab")

section = None
yaml = {}
for line in open("settings/defaults.yaml").read().splitlines():
    if line and not line.startswith(" "):
        section = line.rstrip(":")
    elif line.strip().startswith("timeout:"):
        yaml[section] = line.split(":", 1)[1].strip()
if yaml != {"default": "45", "fallback": "30"}:
    problems.append(f"defaults.yaml timeouts {yaml}, want default 45 and fallback 30")

if problems:
    print("\n".join(problems))
    sys.exit(1)
print("settings ok")
