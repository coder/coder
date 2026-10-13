"""Command line entry point: python3 cli.py <op> <a> <b>."""

import argparse
import sys

import calc

OPERATIONS = {
    "add": calc.add,
    "sub": calc.subtract,
    "mul": calc.multiply,
    "div": calc.divide,
}


def build_parser():
    parser = argparse.ArgumentParser(prog="calc", description="Apply an arithmetic operation to two numbers.")
    parser.add_argument("op", choices=sorted(OPERATIONS))
    parser.add_argument("a", type=float)
    parser.add_argument("b", type=float)
    parser.add_argument("--precision", type=int, default=None, help="Round the result to this many decimal places.")
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    result = OPERATIONS[args.op](args.a, args.b)
    if args.precision is not None:
        result = round(result, args.precision)
    print(result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
