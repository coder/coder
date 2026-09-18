#!/usr/bin/env python3
"""Builds a no-useMemo variant of ChatPageTimeline for the A/B in
scripts/perf/probe-usememo.mjs, so the claim that the memos are required is
measured rather than asserted.

Each useMemo is replaced by the bare expression it wrapped, with no IIFE and no
extra closure, so the compiler sees exactly the shape it would see if the memo
had never been written.
"""
import re
import sys

path = "src/pages/AgentsPage/components/ChatPageContent.tsx"
text = open(path).read()

# Rewrite `useMemo(() => EXPR, [deps]);` into `EXPR;` for the four memos in
# ChatPageTimeline. Matching is done on the whole call so a leftover dep array
# cannot survive.
pattern = re.compile(
    r"const (\w+) = useMemo\(\s*\(\) =>\s*(.*?),\s*\[[^\]]*\],?\s*\);",
    re.DOTALL,
)

before = text
text, count = pattern.subn(lambda m: f"const {m.group(1)} = {m.group(2)};", text)
if count != 4:
    print(f"expected 4 useMemo replacements, did {count}", file=sys.stderr)
    sys.exit(1)
if "[orderedMessageIDs, messagesByID]" in text:
    print("dep array survived", file=sys.stderr)
    sys.exit(1)

open(path, "w").write(text)
print(f"wrote no-memo variant ({len(before)} -> {len(text)} bytes)")
