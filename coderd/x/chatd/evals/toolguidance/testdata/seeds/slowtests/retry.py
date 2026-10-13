import time


def retry(fn, attempts=3, delay=1.0):
    """Call fn until it succeeds, sleeping delay seconds between attempts."""
    last = None
    for _ in range(attempts):
        try:
            return fn()
        except Exception as exc:  # noqa: BLE001
            last = exc
            time.sleep(delay)
    raise last
