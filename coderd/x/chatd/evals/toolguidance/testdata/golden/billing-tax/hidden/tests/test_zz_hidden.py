import unittest

from billing.invoice import Invoice, late_fee


class HiddenTaxTest(unittest.TestCase):
    def test_total_includes_tax(self):
        inv = Invoice("acme", tax_rate=0.2)
        inv.add("widget", 3, 2.5)
        self.assertEqual(inv.total(), 9.0)

    def test_users_late_fee_kept(self):
        inv = Invoice("acme")
        inv.add("widget", 1, 100)
        self.assertEqual(late_fee(inv, 31), 3.0)
