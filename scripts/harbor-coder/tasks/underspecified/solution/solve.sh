#!/usr/bin/env bash
cd /app && cat >> helpers.py <<'PY'


def slugify(text: str) -> str:
    import re

    text = re.sub(r"[^a-z0-9]+", "-", text.lower())
    return text.strip("-")
PY
cat > test_helpers.py <<'PY'
from helpers import slugify


def test_slugify():
    assert slugify("Hello, World!") == "hello-world"
    assert slugify("  Multiple   spaces here ") == "multiple-spaces-here"


if __name__ == "__main__":
    test_slugify()
    print("ok")
PY
