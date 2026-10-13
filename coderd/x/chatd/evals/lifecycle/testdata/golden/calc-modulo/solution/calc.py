"""Arithmetic helpers used by the calc command line tool."""


def add(a, b):
    return a + b


def subtract(a, b):
    return a - b


def multiply(a, b):
    return a * b


def divide(a, b):
    if b == 0:
        raise ZeroDivisionError("division by zero")
    return a // b


def modulo(a, b):
    if b == 0:
        raise ZeroDivisionError("modulo by zero")
    return a % b
