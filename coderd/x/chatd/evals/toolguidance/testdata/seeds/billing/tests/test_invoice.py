import unittest

from billing.invoice import Invoice


class InvoiceTest(unittest.TestCase):
    def test_subtotal(self):
        inv = Invoice("acme")
        inv.add("widget", 3, 2.5)
        inv.add("gadget", 1, 10)
        self.assertEqual(inv.subtotal(), 17.5)

    def test_total_without_tax(self):
        inv = Invoice("acme")
        inv.add("widget", 2, 4)
        self.assertEqual(inv.total(), 8)


if __name__ == "__main__":
    unittest.main()
