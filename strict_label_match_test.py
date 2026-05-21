"""
Strict Label Match Test #2

This test verifies that label matching is strict (case-sensitive and exact).
"""


def strict_label_match(label, target):
    """
    Perform strict label matching.
    
    Args:
        label: The label to check
        target: The target label to match against
        
    Returns:
        bool: True if labels match exactly, False otherwise
    """
    return label == target


def test_exact_match():
    """Test that exact matches return True"""
    assert strict_label_match("bug", "bug") == True
    assert strict_label_match("feature", "feature") == True
    assert strict_label_match("enhancement", "enhancement") == True


def test_case_sensitive():
    """Test that matching is case-sensitive"""
    assert strict_label_match("Bug", "bug") == False
    assert strict_label_match("BUG", "bug") == False
    assert strict_label_match("Feature", "feature") == False


def test_partial_match_fails():
    """Test that partial matches fail"""
    assert strict_label_match("bug-fix", "bug") == False
    assert strict_label_match("bug", "bug-fix") == False
    assert strict_label_match("feature-request", "feature") == False


def test_whitespace_matters():
    """Test that whitespace differences cause mismatch"""
    assert strict_label_match("bug ", "bug") == False
    assert strict_label_match(" bug", "bug") == False
    assert strict_label_match("bug", " bug ") == False


def test_empty_labels():
    """Test handling of empty labels"""
    assert strict_label_match("", "") == True
    assert strict_label_match("bug", "") == False
    assert strict_label_match("", "bug") == False


if __name__ == "__main__":
    test_exact_match()
    test_case_sensitive()
    test_partial_match_fails()
    test_whitespace_matters()
    test_empty_labels()
    print("All strict label match tests passed!")
