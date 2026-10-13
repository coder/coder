import re

UNITS = {"d": 86400, "h": 3600, "m": 60, "s": 1}
TOKEN = re.compile(r"(\d+)([dhms])")


def parse_duration(text):
    """Seconds in a duration such as "1h30m" or "2d4h". Raises ValueError on junk."""
    text = text.strip().lower()
    if not text:
        raise ValueError("empty duration")
    total = 0
    pos = 0
    for match in TOKEN.finditer(text):
        if match.start() != pos:
            raise ValueError(f"invalid duration {text!r}")
        total = int(match.group(1)) * UNITS[match.group(2)]
        pos = match.end()
    if pos != len(text):
        raise ValueError(f"invalid duration {text!r}")
    return total
