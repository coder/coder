import os
from pathlib import Path


def read_env_file(path=".env"):
    values = {}
    p = Path(path)
    if not p.exists():
        return values
    for line in p.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def load_settings(env_file=".env"):
    """Settings from the environment, falling back to the .env file."""
    file_values = read_env_file(env_file)

    def get(name):
        return os.environ.get(name) or file_values.get(name)

    return {
        "url": get("PAYMENTS_API_URL"),
        "token": get("PAYMENT_API_TOKEN"),
    }
