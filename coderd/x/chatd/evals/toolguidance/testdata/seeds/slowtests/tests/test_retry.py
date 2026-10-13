import time
import unittest

from retry import retry


class RetryIntegrationTest(unittest.TestCase):
    def test_scenario_01(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 1, attempts=1), 1)

    def test_scenario_02(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 2, attempts=1), 2)

    def test_scenario_03(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 3, attempts=1), 3)

    def test_scenario_04(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 4, attempts=1), 4)

    def test_scenario_05(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 5, attempts=1), 6)

    def test_scenario_06(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 6, attempts=1), 6)

    def test_scenario_07(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 7, attempts=1), 7)

    def test_scenario_08(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 8, attempts=1), 8)

    def test_scenario_09(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 9, attempts=1), 9)

    def test_scenario_10(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 10, attempts=1), 10)

    def test_scenario_11(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 11, attempts=1), 12)

    def test_scenario_12(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 12, attempts=1), 12)

    def test_scenario_13(self):
        time.sleep(2.8)
        self.assertEqual(retry(lambda: 13, attempts=1), 13)


if __name__ == "__main__":
    unittest.main()
