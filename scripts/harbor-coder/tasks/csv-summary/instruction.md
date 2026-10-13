Write a Python script `summarize.py` in the current directory that reads `sales.csv` and prints per-region revenue totals as JSON.

Requirements:

- Usage: `python summarize.py sales.csv`
- Revenue for a row is `units * unit_price`.
- Output a single JSON object mapping region to total revenue, rounded to 2 decimal places, with keys sorted alphabetically. Regions with zero revenue must still appear.
- Print nothing else to stdout.
- Use only the Python standard library.

Example of the expected shape (values illustrative): `{"east": 1.00, "west": 2.00}`
