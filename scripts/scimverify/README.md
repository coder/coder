# SCIM Verify

> This test harness was set up by an AI agent ([Mux](https://mux.coder.com/))
> during the SCIM 2.0 refactor. It discovered `scimverify`, figured out
> how to run it (the Docker image is a web UI, not the test CLI; use
> `npx scimverify` instead), worked through config quirks (schema detection
> crashes, plural vs singular resource type names, `id: AUTO` targeting the
> admin user), and iterated the config until 14/16 tests passed against a
> live Coder dev server.

Run SCIM 2.0 compliance tests against a live Coder instance using [scimverify](https://verify.scim.dev/).

## Prerequisites

- **Node.js** (npx must be on PATH)
- A running Coder instance with SCIM enabled (`CODER_SCIM_AUTH_HEADER` set)
- An enterprise license (SCIM is an enterprise feature)

## Quick start

```bash
# Start a dev server with SCIM enabled
CODER_SCIM_AUTH_HEADER=my-secret-token ./scripts/develop.sh

# In another terminal, run the tests
./scripts/scimverify/run.sh --token my-secret-token
```

## Usage

```
./scripts/scimverify/run.sh [--base-url URL] [--token TOKEN]

Options:
  --base-url URL   SCIM endpoint base URL (default: http://localhost:3000/scim/v2)
  --token TOKEN    Bearer token matching CODER_SCIM_AUTH_HEADER (required)
  --help           Show help

Environment variables (alternatives to flags):
  SCIM_BASE_URL    Same as --base-url
  SCIM_AUTH_TOKEN  Same as --token
```

## Reading the output

The tool outputs [TAP](https://testanything.org/) (Test Anything Protocol) format:

```
TAP version 13
# Subtest: ResourceTypes
    ok 1 - Retrieves resource types          <-- individual test
ok 1 - ResourceTypes                         <-- suite result
# Subtest: Schemas
    ok 1 - Retrieves schemas
ok 2 - Schemas
# Subtest: Basic tests
    ok 1 - Base URL should not contain any query parameters
    ok 2 - Base URL should be reachable
    ok 3 - Authentication should be required for /Users
ok 3 - Basic tests
# Subtest: Users
    ok 1 - userSchema contains attribute userName ...
    ok 2 - Retrieves a list of users
    not ok 3 - Filters users by userName     <-- FAILURE (details follow)
      ---
      error: |-
        User should have matching userName
        'member' !== 'admin'
      expected: 'admin'
      actual: 'member'
      ...
    ok 4 - Creates a new user - Alternative 1
ok 4 - Users
# tests 16     <-- summary
# pass 14
# fail 1
# skipped 1
```

Key patterns:
- `ok N - description` means the test passed
- `not ok N - description` means the test failed (error details follow in YAML block)
- `ok N - description # SKIP reason` means the test was skipped
- Suite-level `not ok` means at least one subtest in the suite failed
- The summary at the bottom shows total pass/fail/skip counts

### Quick filter for results only

```bash
# Just pass/fail lines
./scripts/scimverify/run.sh --token TOKEN 2>&1 | grep -E "    ok|    not ok"

# Summary only
./scripts/scimverify/run.sh --token TOKEN 2>&1 | grep "^# "
```

## HAR file

Each run writes an [HAR file](https://en.wikipedia.org/wiki/HAR_(file_format))
to `scripts/scimverify/output/output.har` containing every HTTP request and
response. This is useful for debugging failures:

```bash
# Show all requests with errors
python3 -c "
import json
with open('scripts/scimverify/output/output.har') as f:
    har = json.load(f)
for entry in har['log']['entries']:
    req = entry['request']
    resp = entry['response']
    if resp['status'] >= 400:
        body = resp.get('content', {}).get('text', '')
        print(f'{req[\"method\"]} {req[\"url\"]} -> {resp[\"status\"]}')
        if body: print(f'  {body[:200]}')
        print()
"
```

The `output/` directory is gitignored.

## What the tests cover

The test suite is configured via `config.yaml`. Currently it tests:

| Category | Tests | What it checks |
|----------|-------|----------------|
| **ResourceTypes** | 1 | `GET /ResourceTypes` returns 200 with valid resource type definitions |
| **Schemas** | 1 | `GET /Schemas` returns 200 with valid schema definitions |
| **Basic** | 3 | URL format, reachability, and authentication enforcement |
| **Users: Read** | 4 | List users, get single user, non-existent user returns 404, attribute filtering |
| **Users: Create** | 2 | POST two test users with valid SCIM payloads, verify 201 response |
| **Users: Filter** | 1 | Filter by userName (expected to fail; `SupportFiltering: false`) |

### PUT, PATCH, and DELETE

The `run.sh` script works around scimverify's `id: AUTO` limitation (which
resolves to the first user, typically the admin) by pre-creating a sacrificial
test user via `POST /Users`, capturing its UUID, and generating a temporary
config with the real ID hardcoded for PUT, PATCH, and DELETE tests. The test
user has a random name like `scimverify-a1b2c3d4` and is deleted by the
DELETE test at the end of the run.

## Configuration

Edit `config.yaml` to change what gets tested. Key settings:

```yaml
detectSchema: true          # Test /Schemas endpoint
detectResourceTypes: true   # Test /ResourceTypes endpoint
verifyPagination: false     # Skip pagination tests
verifySorting: false        # Skip sorting tests
requireAuthentication: true # Test that unauthenticated requests are rejected

users:
  enabled: true
  operations: [GET, POST, PUT, PATCH, DELETE]  # Which HTTP methods to test
  post_tests:                                   # User creation payloads
    - request: { ... }
  put_tests: []    # Empty = skip (see note above about id: AUTO)
  patch_tests: []  # Empty = skip
  delete_tests: [] # Empty = skip

groups:
  enabled: false   # Coder does not support SCIM groups
```

### Adding test users

To add more POST test cases, append to `post_tests`. Each entry needs a valid
SCIM User payload. Important Coder-specific notes:

- `userName` must be a valid Coder username (lowercase, no `@`, no spaces).
  If invalid, Coder normalizes it (e.g., `user@example.com` becomes `user`).
- `emails` must include at least one entry with `"primary": true`.
- `active` defaults to `true` if omitted.

## Expected results

On a correctly functioning Coder instance with the SCIM 2.0 refactor:

```
# tests 16
# pass 15
# fail 1       <-- filter test (expected, filtering is not supported)
# skipped 0
```

The single expected failure is the filter-by-userName test, because Coder
declares `SupportFiltering: false` in its ServiceProviderConfig.
