import contextlib
import io
import unittest

import cli


def run(argv):
    out = io.StringIO()
    with contextlib.redirect_stdout(out):
        code = cli.main(argv)
    return code, out.getvalue()


class HiddenPrecisionTest(unittest.TestCase):
    def test_rounds_to_precision(self):
        code, out = run(["add", "1.234", "1", "--precision", "2"])
        self.assertEqual(code, 0)
        self.assertEqual(out.strip(), "2.23")

    def test_precision_zero(self):
        _, out = run(["mul", "2.5", "3", "--precision", "0"])
        self.assertIn(out.strip(), ("8", "8.0"))

    def test_default_output_unchanged(self):
        _, out = run(["add", "1.234", "1"])
        self.assertEqual(out.strip(), "2.234")
