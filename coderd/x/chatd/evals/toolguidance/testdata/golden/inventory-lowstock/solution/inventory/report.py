from inventory.pricing import calc_total


def format_price(value):
    return f"${value:.2f}"


def low_stock(items, threshold=5):
    """Items whose quantity is at or below the threshold."""
    return [item for item in items if item.quantity <= threshold]


def summary(items):
    lines = []
    for item in items:
        lines.append(f"{item.sku} {item.name}: {item.quantity} x {format_price(item.unit_price)}")
    lines.append(f"Total value: {format_price(calc_total(items))}")
    return "\n".join(lines)
