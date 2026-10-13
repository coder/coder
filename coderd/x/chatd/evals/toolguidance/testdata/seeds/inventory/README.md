# inventory

Tracks stock items and prints value reports.

- `inventory/models.py`: the `Item` record.
- `inventory/store.py`: loads and saves items as JSON.
- `inventory/pricing.py`: totals and discounts.
- `inventory/report.py`: human-readable reports.

Run the tests with `python3 -m unittest discover -s tests -t .`.
