import unittest

from inventory.models import Item
from inventory.report import low_stock


class HiddenLowStockTest(unittest.TestCase):
    def test_threshold_is_inclusive(self):
        items = [Item("A", "a", 5, 1), Item("B", "b", 4, 1), Item("C", "c", 6, 1)]
        self.assertEqual([i.sku for i in low_stock(items)], ["A", "B"])
        self.assertEqual([i.sku for i in low_stock(items, threshold=6)], ["A", "B", "C"])
