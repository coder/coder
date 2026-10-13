import contextlib
import io
import unittest

import calc
import cli


class HiddenPowerTest(unittest.TestCase):
    def test_power(self):
        self.assertEqual(calc.power(2, 10), 1024)
        self.assertEqual(calc.power(9, 0.5), 3)

    def test_cli_operation(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            cli.main(["pow", "2", "3"])
        self.assertEqual(float(out.getvalue().strip()), 8.0)
