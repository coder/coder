import unittest

import calc


class CalcTest(unittest.TestCase):
    def test_add(self):
        self.assertEqual(calc.add(2, 3), 5)

    def test_subtract(self):
        self.assertEqual(calc.subtract(2, 3), -1)

    def test_multiply(self):
        self.assertEqual(calc.multiply(4, 2.5), 10)

    def test_divide(self):
        self.assertEqual(calc.divide(7, 2), 3.5)

    def test_divide_by_zero(self):
        with self.assertRaises(ZeroDivisionError):
            calc.divide(1, 0)


if __name__ == "__main__":
    unittest.main()
