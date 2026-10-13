from intervals import merge_intervals, total_covered


def test_empty():
    assert merge_intervals([]) == []


def test_disjoint():
    assert merge_intervals([(1, 2), (4, 5)]) == [(1, 2), (4, 5)]


def test_overlapping():
    assert merge_intervals([(1, 3), (2, 6), (8, 10)]) == [(1, 6), (8, 10)]


def test_touching_intervals_merge():
    # Closed intervals that share an endpoint should merge.
    assert merge_intervals([(1, 3), (3, 5)]) == [(1, 5)]


def test_unsorted_input():
    assert merge_intervals([(5, 7), (1, 3), (2, 4)]) == [(1, 4), (5, 7)]


def test_total_covered():
    assert total_covered([(0, 2), (2, 4), (6, 7)]) == 5
