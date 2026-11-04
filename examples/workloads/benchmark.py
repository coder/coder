#!/usr/bin/env python3
"""Measure invocation-to-project-operation latency against a prepared demo host."""

import argparse
import concurrent.futures
import json
import math
import subprocess
import tempfile
import time


COMMAND = """test -s codersdk/client.go && node -e 'if (require("./site/node_modules/lodash").sum([20,22]) !== 42) process.exit(1); console.log("PROJECT_READY")'"""


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--coder", default="coder")
    parser.add_argument("--template", required=True)
    parser.add_argument("--org", required=True)
    parser.add_argument("--cpu", required=True)
    parser.add_argument("--ram", required=True)
    parser.add_argument("--samples", type=int, default=100)
    parser.add_argument("--concurrency", type=int, default=1)
    args = parser.parse_args()
    if args.samples < 1 or args.concurrency < 1:
        parser.error("samples and concurrency must be positive")

    command = [
        args.coder, "run", "--template", args.template, "--org", args.org,
        "--cpu", args.cpu, "--ram", args.ram, "--cmd", COMMAND,
    ]

    def run(_):
        # A file prevents diagnostics from filling an unread stderr pipe.
        with tempfile.TemporaryFile() as errors:
            start = time.perf_counter()
            with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=errors) as proc:
                ready = None
                for line in proc.stdout:
                    if line.strip() == b"PROJECT_READY" and ready is None:
                        ready = time.perf_counter() - start
                code = proc.wait()
            if code != 0 or ready is None:
                errors.seek(0)
                return {"exit_code": code, "error": errors.read(16384).decode(errors="replace")}
            return {"ready_ms": ready * 1000}

    with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
        results = list(pool.map(run, range(args.samples)))
    durations = sorted(result["ready_ms"] for result in results if "ready_ms" in result)
    failures = [result for result in results if "error" in result]
    report = {
        "samples": args.samples, "concurrency": args.concurrency,
        "successful": len(durations), "failures": failures,
        "measurement": "new CLI process to observed source/dependency operation marker",
        "conditions": "prepared image; cache warmth and network distance are operator-controlled",
    }
    if durations:
        report["ready_ms"] = {
            f"p{p}": durations[math.ceil(len(durations) * p / 100) - 1]
            for p in (50, 95, 99)
        }
        report["ready_ms"]["max"] = durations[-1]
    print(json.dumps(report, indent=2))
    return bool(failures)


if __name__ == "__main__":
    raise SystemExit(main())
