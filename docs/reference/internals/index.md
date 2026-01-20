# Internals Reference

This section provides technical documentation for Coder's internal systems. These documents are intended for advanced users, template authors, and contributors who need to understand how Coder works under the hood.

## Contents

- [Terraform State Management](./terraform-state-management.md) - How Coder manages Terraform state across workspace lifecycle operations, including state flow, update conditions, and the stale state edge case.

## Audience

These documents assume familiarity with:

- Coder's workspace and template concepts
- Terraform fundamentals
- Go programming (for source code references)
- Distributed systems concepts

## Contributing

If you find inaccuracies or have suggestions for additional internals documentation, please open an issue or pull request on the [Coder repository](https://github.com/coder/coder).
