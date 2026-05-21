# Strict Label Match Reverse Alice Test Suite

This directory contains test files for strict label matching functionality, specifically focused on reverse lookup operations for user-based labels.

## Overview

The `strict_label_match_reverse_alice_test.py` module tests:

1. **Strict Label Matching**: Ensures labels match exactly (case-sensitive, no partial matches)
2. **Reverse Lookup**: Given a username (e.g., "alice"), find all labels associated with that user
3. **Label Consistency**: Verifies that forward and reverse lookups produce consistent results

## Test Coverage

### Strict Matching Tests
- ✅ Exact label matches (e.g., `owner:alice`)
- ✅ Case-sensitive matching (e.g., `owner:Alice` ≠ `owner:alice`)
- ✅ Partial match rejection (e.g., `owner:ali` ≠ `owner:alice`)
- ✅ Prefix match rejection (e.g., `owner:` ≠ `owner:alice`)
- ✅ Whitespace sensitivity (e.g., `owner:alice ` ≠ `owner:alice`)
- ✅ Empty label handling

### Reverse Lookup Tests
- ✅ Find all labels for a specific user
- ✅ Exclude labels from other users
- ✅ Support multiple role-based labels per user
- ✅ Maintain forward/reverse lookup consistency

## Running Tests

Run the test suite with:

```bash
python3 tests/strict_label_match_reverse_alice_test.py
```

Expected output:
```
test_reverse_lookup_labels_function ... ok
test_strict_label_match_function ... ok
test_case_sensitive_matching ... ok
test_empty_label_handling ... ok
test_exact_label_match_alice ... ok
test_multiple_role_labels_for_alice ... ok
test_reverse_lookup_alice_labels ... ok
test_reverse_lookup_excludes_other_users ... ok
test_reverse_match_consistency ... ok
test_strict_no_partial_match ... ok
test_strict_no_prefix_match ... ok
test_whitespace_sensitivity ... ok

----------------------------------------------------------------------
Ran 12 tests in 0.000s

OK
```

## Use Cases

This functionality is relevant for:

- **Search Query Parsing**: When users search with labels like `owner:alice`
- **Access Control**: Verifying label-based permissions
- **Audit Trails**: Tracking label assignments and ownership
- **User Dashboards**: Displaying all labels/roles associated with a user

## Implementation Notes

The test suite includes two helper functions:

1. `strict_label_match(query_label, available_labels)`: Performs exact matching
2. `reverse_lookup_labels(username, label_data)`: Finds all labels for a user

These functions demonstrate the expected behavior for production implementation in the Coder search query system (see `coderd/searchquery/search.go` and `coderd/searchquery/search_test.go`).

## Related Code

- `coderd/searchquery/search.go` - Search query parsing and filtering
- `coderd/searchquery/search_test.go` - Existing search functionality tests
- Particularly the `TestSearchTasks` function with `owner:alice` test case (line 1044)
