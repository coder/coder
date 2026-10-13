import unittest

from shop.format import format_price


class FormatTest(unittest.TestCase):
    def test_small(self):
        self.assertEqual(format_price(250), "$2.50")

    def test_currency(self):
        self.assertEqual(format_price(5, currency="EUR "), "EUR 0.05")


if __name__ == "__main__":
    unittest.main()
