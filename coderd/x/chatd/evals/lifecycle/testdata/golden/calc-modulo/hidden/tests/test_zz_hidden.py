import contextlib
import io
import unittest

import calc
import cli


class HiddenModuloTest(unittest.TestCase):
    def test_modulo(self):
        self.assertEqual(calc.modulo(7, 3), 1)
        self.assertEqual(calc.modulo(7.5, 2), 1.5)

    def test_cli_operation(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            cli.main(["mod", "10", "4"])
        self.assertEqual(float(out.getvalue().strip()), 2.0)
