from dataclasses import dataclass, field


@dataclass
class Line:
    description: str
    quantity: int
    unit_price: float


@dataclass
class Invoice:
    customer: str
    tax_rate: float = 0.0
    lines: list = field(default_factory=list)

    def add(self, description, quantity, unit_price):
        self.lines.append(Line(description, quantity, unit_price))

    def subtotal(self):
        return sum(line.quantity * line.unit_price for line in self.lines)

    def total(self):
        """Subtotal plus tax at tax_rate (0.2 means 20 percent)."""
        return round(self.subtotal(), 2)


def late_fee(invoice, days_late):
    """Work in progress: 1.5 percent of the total per started 30 days."""
    if days_late <= 0:
        return 0.0
    periods = (days_late + 29) // 30
    return round(invoice.total() * 0.015 * periods, 2)
