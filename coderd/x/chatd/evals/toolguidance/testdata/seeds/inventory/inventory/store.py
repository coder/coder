import json
from pathlib import Path

from inventory.models import Item


class Store:
    """Keeps items in memory and persists them to a JSON file."""

    def __init__(self, path):
        self.path = Path(path)
        self.items = {}

    def load(self):
        if not self.path.exists():
            self.items = {}
            return
        data = json.loads(self.path.read_text())
        self.items = {row["sku"]: Item(**row) for row in data["items"]}

    def save(self):
        rows = [vars(item) for item in sorted(self.items.values(), key=lambda item: item.sku)]
        tmp = self.path.with_suffix(".tmp")
        tmp.write_text(json.dumps({"items": rows}, indent=2))
        tmp.replace(self.path)

    def add(self, item):
        self.items[item.sku] = item

    def remove(self, sku):
        del self.items[sku]
