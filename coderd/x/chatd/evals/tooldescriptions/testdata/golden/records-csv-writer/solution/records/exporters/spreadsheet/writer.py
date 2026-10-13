import csv
import io


def write_rows(rows):
    """CSV text for a list of dicts, with a header row from the first row's keys."""
    if not rows:
        return ""
    out = io.StringIO()
    writer = csv.DictWriter(out, fieldnames=list(rows[0]))
    writer.writeheader()
    writer.writerows(rows)
    return out.getvalue()
