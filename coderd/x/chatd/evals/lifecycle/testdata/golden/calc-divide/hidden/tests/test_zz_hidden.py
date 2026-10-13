import unittest

import calc


class HiddenDivideTest(unittest.TestCase):
    def test_true_division(self):
        self.assertEqual(calc.divide(7, 2), 3.5)
        self.assertEqual(calc.divide(-7, 2), -3.5)
        self.assertEqual(calc.divide(1, 4), 0.25)

    def test_zero(self):
        with self.assertRaises(ZeroDivisionError):
            calc.divide(3, 0)
