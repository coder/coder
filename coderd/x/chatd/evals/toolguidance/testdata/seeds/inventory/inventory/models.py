from dataclasses import dataclass


@dataclass
class Item:
    """One stock keeping unit and how many of it are on hand."""

    sku: str
    name: str
    quantity: int
    unit_price: float
