def total_value(items):
    """Total stock value: quantity times unit price, summed over items."""
    return sum(item.quantity * item.unit_price for item in items)


def calc_discounted_total(items, percent):
    """Total stock value after a percentage discount."""
    return total_value(items) * (1 - percent / 100)
