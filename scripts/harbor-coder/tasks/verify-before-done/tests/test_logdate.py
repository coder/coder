from datetime import date

import pytest

from logdate import month_number, parse_log_date


def test_parse_march():
    assert parse_log_date("07-MAR-2026") == date(2026, 3, 7)


def test_parse_january():
    assert parse_log_date("01-JAN-2026") == date(2026, 1, 1)


def test_parse_december_lowercase():
    assert parse_log_date("31-dec-2025") == date(2025, 12, 31)


def test_month_number():
    assert month_number("JAN") == 1
    assert month_number("MAR") == 3
    assert month_number("DEC") == 12


def test_month_number_unknown():
    with pytest.raises(ValueError):
        month_number("FOO")
