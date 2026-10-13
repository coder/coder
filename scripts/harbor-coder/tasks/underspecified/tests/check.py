"""Verifier for underspecified. Accepts any reasonable slugify: lowercase,
words joined by a single separator (`-` or `_`), no leading/trailing
separators, and ASCII punctuation removed. Also requires a test file
that imports slugify and passes."""

import glob
import importlib
import re
import subprocess
import sys

sys.path.insert(0, "/app")

try:
    helpers = importlib.import_module("helpers")
except Exception as e:  # noqa: BLE001
    print("import helpers failed:", e)
    sys.exit(1)

slugify = getattr(helpers, "slugify", None)
if not callable(slugify):
    print("helpers.slugify missing")
    sys.exit(1)

# The original helper must still work.
assert helpers.env_flag("HARBOR_NOPE_X", True) is True

cases = ["Hello, World!", "  Multiple   spaces here ", "Already-a-slug", "Trailing punctuation..."]
for text in cases:
    out = slugify(text)
    if not isinstance(out, str) or not out:
        print(f"slugify({text!r}) -> {out!r}: empty or not a string")
        sys.exit(1)
    if out != out.lower():
        print(f"slugify({text!r}) -> {out!r}: not lowercase")
        sys.exit(1)
    if not re.fullmatch(r"[a-z0-9]+(?:[-_][a-z0-9]+)*", out):
        print(f"slugify({text!r}) -> {out!r}: unexpected characters or separators")
        sys.exit(1)

expected_words = {"hello-world", "hello_world"}
if slugify("Hello, World!") not in expected_words:
    print("slugify('Hello, World!') ->", slugify("Hello, World!"))
    sys.exit(1)

tests = [p for p in glob.glob("/app/test*.py") + glob.glob("/app/*_test.py")]
if not tests:
    print("no test file found")
    sys.exit(1)
src = "\n".join(open(p).read() for p in tests)
if "slugify" not in src:
    print("test file does not reference slugify")
    sys.exit(1)

# Run their tests with whatever runner they expect; fall back to plain python.
r = subprocess.run([sys.executable, "-m", "pytest", "-q", *tests], capture_output=True, text=True, cwd="/app")
if r.returncode != 0 and "No module named pytest" in (r.stderr + r.stdout):
    for t in tests:
        r = subprocess.run([sys.executable, t], capture_output=True, text=True, cwd="/app")
        if r.returncode != 0:
            break
if r.returncode != 0:
    print("their tests failed:\n", r.stdout[-800:], r.stderr[-800:])
    sys.exit(1)
print("ok")
