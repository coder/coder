"""Processes one job per second until interrupted."""

import signal
import sys
import time

running = True


def stop(signum, frame):
    global running
    running = False


signal.signal(signal.SIGTERM, stop)
signal.signal(signal.SIGINT, stop)

job = 0
while running:
    time.sleep(1)
    job += 1
    print(f"processed job {job}", flush=True)
print("worker stopped", flush=True)
sys.exit(0)
