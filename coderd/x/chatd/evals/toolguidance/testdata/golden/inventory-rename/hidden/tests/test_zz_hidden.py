import unittest

from inventory import pricing, report
from inventory.models import Item


class HiddenRenameTest(unittest.TestCase):
    def test_total_value(self):
        items = [Item("A1", "bolt", 10, 0.25), Item("B2", "nut", 4, 0.1)]
        self.assertAlmostEqual(pricing.total_value(items), 2.9)
        self.assertFalse(hasattr(pricing, "calc_" + "total"))
        self.assertIn("Total value: $2.90", report.summary(items))
