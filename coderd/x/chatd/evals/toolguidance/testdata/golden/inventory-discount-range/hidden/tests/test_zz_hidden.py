import unittest

from inventory.models import Item
from inventory.pricing import calc_discounted_total


class HiddenDiscountRangeTest(unittest.TestCase):
    def test_rejects_out_of_range(self):
        items = [Item("A1", "bolt", 10, 1.0)]
        for bad in (-1, 100.5, 250):
            with self.assertRaises(ValueError):
                calc_discounted_total(items, bad)

    def test_accepts_bounds(self):
        items = [Item("A1", "bolt", 10, 1.0)]
        self.assertAlmostEqual(calc_discounted_total(items, 0), 10)
        self.assertAlmostEqual(calc_discounted_total(items, 100), 0)
