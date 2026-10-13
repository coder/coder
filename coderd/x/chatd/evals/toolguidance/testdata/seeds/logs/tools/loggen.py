"""Writes logs/app.log, a deterministic ~30 MB service log, for the large-log case."""

import os
import random

rng = random.Random(20261013)
services = ["auth", "payments", "search", "orders", "billing", "gateway"]
levels = ["INFO"] * 14 + ["WARN"] * 4 + ["ERROR"] * 2
codes = ["E1001", "E2002", "E4012", "E4013", "E5000"]
messages = [
    "request completed",
    "cache miss for key",
    "upstream responded slowly",
    "retrying request",
    "validation failed for field",
    "connection reset by peer",
]
os.makedirs("logs", exist_ok=True)
with open("logs/app.log", "w") as f:
    for i in range(280000):
        day = 1 + i * 28 // 280000
        second = i % 86400
        level = rng.choice(levels)
        service = rng.choice(services)
        code = rng.choice(codes) if level == "ERROR" else "-"
        request = rng.getrandbits(64)
        message = rng.choice(messages)
        f.write(
            f"2026-09-{day:02d}T{second // 3600:02d}:{second // 60 % 60:02d}:{second % 60:02d}Z "
            f"{level:<5} service={service} code={code} request_id={request:016x} msg=\"{message}\"\n"
        )
