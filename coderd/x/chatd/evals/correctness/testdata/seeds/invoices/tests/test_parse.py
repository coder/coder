import unittest
from decimal import Decimal

from invoices.parse import parse_record


class ParseTest(unittest.TestCase):
    def test_record(self):
        inv = parse_record({"number": " A-1 ", "amount": "10.00", "currency": "eur"})
        self.assertEqual((inv.number, inv.amount, inv.currency), ("A-1", Decimal("10.00"), "EUR"))


if __name__ == "__main__":
    unittest.main()
