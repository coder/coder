import unittest

from durations import parse_duration


class HiddenDurationTest(unittest.TestCase):
    def test_combinations(self):
        self.assertEqual(parse_duration("2d4h"), 2 * 86400 + 4 * 3600)
        self.assertEqual(parse_duration("1h1m1s"), 3661)
        self.assertEqual(parse_duration(" 10M "), 600)

    def test_still_rejects_junk(self):
        for bad in ("", "1x", "h1", "1h 30m"):
            with self.assertRaises(ValueError):
                parse_duration(bad)
