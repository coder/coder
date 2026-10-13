import csv
import io
import unittest

from records.exporters.csv.writer import write_rows


class HiddenWriterTest(unittest.TestCase):
    def test_header_from_first_row(self):
        text = write_rows([{"id": 1, "name": "Ada"}, {"id": 2, "name": "Grace, R."}])
        rows = list(csv.reader(io.StringIO(text)))
        self.assertEqual(rows, [["id", "name"], ["1", "Ada"], ["2", "Grace, R."]])

    def test_empty(self):
        self.assertEqual(write_rows([]), "")
