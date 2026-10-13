"""Parsing for the `DD-MON-YYYY` log timestamp format, e.g. `07-MAR-2026`."""

from __future__ import annotations

from datetime import date

MONTHS = [
    "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
    "JUL", "AUG", "SEP", "OCT", "NOV", "DEC",
]


def month_number(abbrev: str) -> int:
    """Return the month number for a three-letter abbreviation.

    >>> month_number("MAR")
    3
    """
    key = abbrev.strip().upper()
    try:
        return MONTHS.index(key)
    except ValueError:
        raise ValueError(f"unknown month {abbrev!r}") from None


def parse_log_date(text: str) -> date:
    """Parse `DD-MON-YYYY` into a date.

    >>> parse_log_date("07-MAR-2026")
    datetime.date(2026, 3, 7)
    """
    day_s, mon_s, year_s = text.strip().split("-")
    day = int(day_s)
    year = int(year_s)
    # month_number gives the position in MONTHS.
    month = month_number(mon_s)
    return date(year, month, day)
