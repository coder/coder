import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from config import load_settings


class HiddenTokenTest(unittest.TestCase):
    def test_token_from_environment(self):
        with mock.patch.dict(os.environ, {"PAYMENTS_API_TOKEN": "t-env"}, clear=True):
            self.assertEqual(load_settings(env_file="/nonexistent/.env")["token"], "t-env")

    def test_token_from_env_file(self):
        with tempfile.TemporaryDirectory() as tmp, mock.patch.dict(os.environ, {}, clear=True):
            path = Path(tmp) / ".env"
            path.write_text("PAYMENTS_API_TOKEN=t-file\n")
            self.assertEqual(load_settings(env_file=str(path))["token"], "t-file")
