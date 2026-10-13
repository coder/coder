from dataclasses import dataclass
from decimal import Decimal


@dataclass
class Invoice:
    number: str
    amount: Decimal
    currency: str


def parse_record(record):
    """Builds an Invoice from one exported record dict."""
    number = str(record["number"]).strip()
    amount = Decimal(str(record["amount"]))
    currency = record.get("currency", "USD").upper()
    return Invoice(number, amount, currency)


def parse_all(records):
    return [parse_record(r) for r in records]
