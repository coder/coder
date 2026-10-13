import unittest

from durations import parse_duration


class DurationTest(unittest.TestCase):
    def test_single_unit(self):
        self.assertEqual(parse_duration("90s"), 90)

    def test_combined(self):
        self.assertEqual(parse_duration("1h30m"), 5400)

    def test_junk(self):
        with self.assertRaises(ValueError):
            parse_duration("soon")


if __name__ == "__main__":
    unittest.main()
