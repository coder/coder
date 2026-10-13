import unittest

from shop.format import format_price


class HiddenThousandsTest(unittest.TestCase):
    def test_separator(self):
        self.assertEqual(format_price(123450), "$1,234.50")
        self.assertEqual(format_price(100000000), "$1,000,000.00")
        self.assertEqual(format_price(99999), "$999.99")
