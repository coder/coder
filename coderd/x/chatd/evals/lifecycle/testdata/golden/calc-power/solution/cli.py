"""Command line entry point: python3 cli.py <op> <a> <b>."""

import argparse
import sys

import calc

OPERATIONS = {
    "add": calc.add,
    "sub": calc.subtract,
    "mul": calc.multiply,
    "div": calc.divide,
    "pow": calc.power,
}


def build_parser():
    parser = argparse.ArgumentParser(prog="calc", description="Apply an arithmetic operation to two numbers.")
    parser.add_argument("op", choices=sorted(OPERATIONS))
    parser.add_argument("a", type=float)
    parser.add_argument("b", type=float)
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    result = OPERATIONS[args.op](args.a, args.b)
    print(result)
    return 0


if __name__ == "__main__":
    sys.exit(main())
