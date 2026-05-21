#!/usr/bin/env python3
"""
Strict Label Match Reverse Alice Test

This test suite verifies strict label matching with reverse lookup functionality,
specifically for user-based label queries (e.g., owner:alice).

Test scenarios:
- Exact label matching (case-sensitive)
- Reverse lookup from user to labels
- Label ownership verification
"""

import unittest


class StrictLabelMatchReverseAliceTest(unittest.TestCase):
    """Test suite for strict label matching with reverse alice lookup."""

    def setUp(self):
        """Set up test data for alice user and labels."""
        self.alice_labels = {
            "owner:alice": {"user": "alice", "role": "owner"},
            "assignee:alice": {"user": "alice", "role": "assignee"},
            "reviewer:alice": {"user": "alice", "role": "reviewer"},
        }
        
        self.all_labels = {
            "owner:alice": {"user": "alice", "role": "owner"},
            "owner:bob": {"user": "bob", "role": "owner"},
            "assignee:alice": {"user": "alice", "role": "assignee"},
            "assignee:charlie": {"user": "charlie", "role": "assignee"},
        }

    def test_exact_label_match_alice(self):
        """Test that exact label 'owner:alice' matches strictly."""
        label = "owner:alice"
        self.assertIn(label, self.alice_labels)
        self.assertEqual(self.alice_labels[label]["user"], "alice")

    def test_case_sensitive_matching(self):
        """Test that label matching is case-sensitive."""
        # "owner:Alice" (capital A) should not match "owner:alice"
        label_upper = "owner:Alice"
        self.assertNotIn(label_upper, self.alice_labels)

    def test_reverse_lookup_alice_labels(self):
        """Test reverse lookup: find all labels belonging to alice."""
        alice_user_labels = [
            label for label, data in self.all_labels.items()
            if data["user"] == "alice"
        ]
        expected_labels = ["owner:alice", "assignee:alice"]
        self.assertEqual(sorted(alice_user_labels), sorted(expected_labels))

    def test_reverse_lookup_excludes_other_users(self):
        """Test that reverse lookup for alice excludes other users."""
        alice_user_labels = [
            label for label, data in self.all_labels.items()
            if data["user"] == "alice"
        ]
        # Ensure bob's and charlie's labels are not included
        self.assertNotIn("owner:bob", alice_user_labels)
        self.assertNotIn("assignee:charlie", alice_user_labels)

    def test_strict_no_partial_match(self):
        """Test that partial matches are rejected."""
        # "owner:ali" should not match "owner:alice"
        partial = "owner:ali"
        self.assertNotIn(partial, self.alice_labels)

    def test_strict_no_prefix_match(self):
        """Test that prefix matches are rejected."""
        # "owner:" should not match "owner:alice"
        prefix_only = "owner:"
        self.assertNotIn(prefix_only, self.alice_labels)

    def test_whitespace_sensitivity(self):
        """Test that labels with whitespace are distinct."""
        # "owner:alice " (with trailing space) should not match "owner:alice"
        label_with_space = "owner:alice "
        self.assertNotIn(label_with_space, self.alice_labels)

    def test_empty_label_handling(self):
        """Test that empty labels are handled correctly."""
        empty_label = ""
        self.assertNotIn(empty_label, self.alice_labels)

    def test_multiple_role_labels_for_alice(self):
        """Test that alice can have multiple role-based labels."""
        alice_roles = [
            data["role"] for label, data in self.alice_labels.items()
            if data["user"] == "alice"
        ]
        expected_roles = ["owner", "assignee", "reviewer"]
        self.assertEqual(sorted(alice_roles), sorted(expected_roles))

    def test_reverse_match_consistency(self):
        """Test that forward and reverse lookups are consistent."""
        # Forward: label -> user
        label = "owner:alice"
        self.assertEqual(self.alice_labels[label]["user"], "alice")
        
        # Reverse: user -> labels
        alice_labels = [
            label for label, data in self.alice_labels.items()
            if data["user"] == "alice"
        ]
        self.assertIn(label, alice_labels)


def strict_label_match(query_label: str, available_labels: set) -> bool:
    """
    Perform strict label matching.
    
    Args:
        query_label: The label to search for
        available_labels: Set of available labels
        
    Returns:
        True if exact match found, False otherwise
    """
    return query_label in available_labels


def reverse_lookup_labels(username: str, label_data: dict) -> list:
    """
    Perform reverse lookup: find all labels for a given user.
    
    Args:
        username: The username to search for
        label_data: Dictionary mapping labels to user data
        
    Returns:
        List of labels belonging to the user
    """
    return [
        label for label, data in label_data.items()
        if data.get("user") == username
    ]


class StrictLabelMatchFunctionsTest(unittest.TestCase):
    """Test the helper functions for label matching."""

    def test_strict_label_match_function(self):
        """Test the strict_label_match function."""
        labels = {"owner:alice", "assignee:bob"}
        
        # Exact match
        self.assertTrue(strict_label_match("owner:alice", labels))
        
        # No match
        self.assertFalse(strict_label_match("owner:Alice", labels))
        self.assertFalse(strict_label_match("owner:ali", labels))

    def test_reverse_lookup_labels_function(self):
        """Test the reverse_lookup_labels function."""
        label_data = {
            "owner:alice": {"user": "alice"},
            "owner:bob": {"user": "bob"},
            "assignee:alice": {"user": "alice"},
        }
        
        alice_labels = reverse_lookup_labels("alice", label_data)
        self.assertEqual(len(alice_labels), 2)
        self.assertIn("owner:alice", alice_labels)
        self.assertIn("assignee:alice", alice_labels)


if __name__ == "__main__":
    # Run all tests
    unittest.main(verbosity=2)
