"""Tiny interval utilities."""

from __future__ import annotations


def merge_intervals(intervals: list[tuple[int, int]]) -> list[tuple[int, int]]:
    """Merge overlapping or touching closed intervals.

    >>> merge_intervals([(1, 3), (2, 6), (8, 10)])
    [(1, 6), (8, 10)]
    """
    if not intervals:
        return []
    ordered = sorted(intervals)
    merged = [ordered[0]]
    for start, end in ordered[1:]:
        last_start, last_end = merged[-1]
        if start < last_end:
            merged[-1] = (last_start, max(last_end, end))
        else:
            merged.append((start, end))
    return merged


def total_covered(intervals: list[tuple[int, int]]) -> int:
    """Total length covered by the union of the intervals."""
    return sum(end - start for start, end in merge_intervals(intervals))
