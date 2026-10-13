import csv
import io
import unittest

from inventory.csv_export import to_csv
from inventory.models import Item


class HiddenCSVTest(unittest.TestCase):
    def test_rows(self):
        text = to_csv([Item("A1", "bolt, steel", 10, 0.25), Item("B2", "nut", 4, 0.1)])
        rows = list(csv.reader(io.StringIO(text)))
        self.assertEqual(rows[0], ["sku", "name", "quantity", "unit_price"])
        self.assertEqual(rows[1], ["A1", "bolt, steel", "10", "0.25"])
        self.assertEqual(rows[2], ["B2", "nut", "4", "0.1"])
        self.assertEqual(len(rows), 3)

    def test_empty(self):
        rows = list(csv.reader(io.StringIO(to_csv([]))))
        self.assertEqual(rows, [["sku", "name", "quantity", "unit_price"]])
