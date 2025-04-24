# Documentation Workflow Testing

This directory contains utilities for testing the documentation validation workflows locally.

## Vale Testing

The `test-vale.sh` script is designed to validate that the Vale integration in the unified documentation workflow functions correctly. This script simulates the GitHub Actions environment and tests the full Vale style checking approach, including:

1. Installation of Vale using the same method as the workflow
2. Basic Vale execution and verification
3. JSON output format validation
4. Chunked processing of multiple files (as implemented in the workflow)

### Running the Test

To run the Vale integration test:

```bash
cd .github/docs/testing
./test-vale.sh
```

### What the Test Covers

- Validates that Vale can be properly installed and run
- Confirms that Vale produces valid JSON output
- Tests the chunked processing approach used in the workflow
- Verifies the JSON results combination logic

### When to Run This Test

Run this test when:

1. Making changes to the Vale integration in the docs-core action
2. Upgrading the Vale version used in the workflow
3. Modifying the Vale configuration or style rules
4. Troubleshooting Vale-related issues in the GitHub Actions environment

## Implementation Notes

The Vale integration in our workflow has two components:

1. Installation in the parent workflow (`docs-unified.yaml`)
2. Execution in the docs-core composite action

This approach ensures that Vale is properly installed and accessible to the composite action without requiring it to download and install Vale itself, which could be unreliable within the composite action context.