import csv
import io


def to_csv(items):
    """CSV text with a header row and one row per item."""
    out = io.StringIO()
    writer = csv.writer(out)
    writer.writerow(["sku", "name", "quantity", "unit_price"])
    for item in items:
        writer.writerow([item.sku, item.name, item.quantity, item.unit_price])
    return out.getvalue()
