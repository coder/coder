import unittest

from records.normalize import normalize_name, normalize_record


class HiddenNameTest(unittest.TestCase):
    def test_empty_names(self):
        for value in (None, "", "   "):
            self.assertEqual(normalize_name(value), "unknown")

    def test_whitespace(self):
        self.assertEqual(normalize_name("  ada   LOVELACE "), "Ada Lovelace")
        self.assertEqual(normalize_record({"name": " grace  hopper"}), {"name": "Grace Hopper"})
