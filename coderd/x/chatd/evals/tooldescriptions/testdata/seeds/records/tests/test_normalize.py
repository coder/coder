import unittest

from records.normalize import normalize_name, normalize_record


class NormalizeTest(unittest.TestCase):
    def test_name(self):
        self.assertEqual(normalize_name("ada lovelace"), "Ada Lovelace")

    def test_record_keeps_unknown_fields(self):
        self.assertEqual(normalize_record({"zzz": 1}), {"zzz": 1})


if __name__ == "__main__":
    unittest.main()
