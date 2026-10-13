import tempfile
import unittest
from pathlib import Path

from config import read_env_file


class ConfigTest(unittest.TestCase):
    def test_read_env_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / ".env"
            path.write_text("# comment\nA=1\nB = two\n")
            self.assertEqual(read_env_file(path), {"A": "1", "B": "two"})


if __name__ == "__main__":
    unittest.main()
