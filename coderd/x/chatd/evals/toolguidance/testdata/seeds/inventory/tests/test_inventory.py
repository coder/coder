import tempfile
import unittest
from pathlib import Path

from inventory.models import Item
from inventory.pricing import calc_discounted_total, calc_total
from inventory.report import format_price, summary
from inventory.store import Store


def sample():
    return [Item("A1", "bolt", 10, 0.25), Item("B2", "nut", 4, 0.1)]


class InventoryTest(unittest.TestCase):
    def test_total(self):
        self.assertAlmostEqual(calc_total(sample()), 2.9)

    def test_discount(self):
        self.assertAlmostEqual(calc_discounted_total(sample(), 10), 2.61)

    def test_format_price(self):
        self.assertEqual(format_price(3.5), "$3.50")

    def test_summary(self):
        self.assertIn("Total value: $2.90", summary(sample()))

    def test_store_round_trip(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "items.json"
            store = Store(path)
            for item in sample():
                store.add(item)
            store.save()
            loaded = Store(path)
            loaded.load()
            self.assertEqual(sorted(loaded.items), ["A1", "B2"])


if __name__ == "__main__":
    unittest.main()
