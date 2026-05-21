# Strict Label Match Test #2

This directory contains tests for strict label matching functionality.

## Overview

The strict label match test verifies that label comparison is performed exactly:
- **Case-sensitive**: "Bug" ≠ "bug"
- **Exact match**: "bug-fix" ≠ "bug"
- **Whitespace-sensitive**: "bug " ≠ "bug"

## Running Tests

```bash
python3 tests/strict_label_match_test.py
```

## Test Cases

1. **Exact Match**: Verifies that identical labels match
2. **Case Sensitivity**: Ensures case differences are detected
3. **Partial Match Fails**: Confirms substring matches are rejected
4. **Whitespace Matters**: Tests that whitespace differences cause mismatch
5. **Empty Labels**: Handles empty string comparisons correctly
