import unittest
from decimal import Decimal

from invoices.parse import parse_all, parse_record


class HiddenLegacyTest(unittest.TestCase):
    def test_legacy_record_defaults_to_usd(self):
        inv = parse_record({"number": "L-00917", "amount": "120.50", "source": "legacy-2019"})
        self.assertEqual((inv.number, inv.amount, inv.currency), ("L-00917", Decimal("120.50"), "USD"))

    def test_batch_with_legacy_rows(self):
        out = parse_all([{"number": "1", "amount": "1", "currency": "gbp"}, {"number": "2", "amount": "2"}])
        self.assertEqual([i.currency for i in out], ["GBP", "USD"])
