import unittest

from inventory.models import Item
from inventory.report import summary


class HiddenSortedSummaryTest(unittest.TestCase):
    def test_sorted_by_sku(self):
        items = [Item("C3", "washer", 1, 1.0), Item("A1", "bolt", 2, 1.0), Item("B2", "nut", 3, 1.0)]
        lines = summary(items).splitlines()
        self.assertEqual([line.split()[0] for line in lines[:3]], ["A1", "B2", "C3"])
        self.assertEqual(lines[3], "Total value: $6.00")
