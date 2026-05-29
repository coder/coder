# SCIM Verify

Run SCIM 2.0 compliance tests against a live Coder instance using
[scimverify](https://verify.scim.dev/). The wrapper script pre-creates a
dedicated test user (so PUT/PATCH/DELETE do not target the admin), generates
a temporary config, runs the full suite, and adds a curl check that the
upstream suite does not cover (recreate-after-delete reactivates the
existing row instead of conflicting or duplicating).

## Prerequisites

- **Node.js** (npx must be on PATH)
- **jq** (used by `run.sh` to parse SCIM responses)
- A running Coder instance with SCIM enabled (`CODER_SCIM_AUTH_HEADER` set)
- An enterprise license loaded; SCIM is gated by `FeatureSCIM`. Load one
  with `coder licenses add -f <path-to-license.jwt>` or `coder licenses
  add -l <license-string>`. See
  [docs/admin/licensing](../../docs/admin/licensing/index.md) for the
  full setup, including how to request a trial.

## Quick start

```bash
# Start a dev server with SCIM enabled.
CODER_SCIM_AUTH_HEADER=my-secret-token ./scripts/develop.sh

# In a second terminal, point the CLI at the dev server and load a license.
export CODER_URL=http://localhost:3000
coder licenses add -f /path/to/license.jwt

# Run the suite.
./scripts/scimverify/run.sh --token my-secret-token
```

If SCIM is not enabled the script exits early with a 404 from
`/scim/v2/Users`. If SCIM is enabled but no license is loaded it exits
early with a 403 ("SCIM is a Premium feature").

## Usage

```text
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

```text
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

The test suite is configured by the heredoc inside `run.sh` (which writes a
temporary config file each run, because PUT/PATCH/DELETE need the freshly
created test user's UUID injected). Currently it tests:

| Category          | Tests | What it checks                                                                  |
|-------------------|-------|---------------------------------------------------------------------------------|
| **ResourceTypes** | 1     | `GET /ResourceTypes` returns 200 with valid resource type definitions           |
| **Schemas**       | 1     | `GET /Schemas` returns 200 with valid schema definitions                        |
| **Basic**         | 3     | URL format, reachability, and authentication enforcement                        |
| **Users: Read**   | 6     | List users, get single user, non-existent user returns 404, attribute filtering |
| **Users: Create** | 5     | Standard, minimal body, `active: false` initial state, and multi-email payloads |
| **Users: PUT**    | 2     | Rename via PUT, then suspend via PUT                                            |
| **Users: PATCH**  | 3     | Suspend with path, reactivate via path-less op, suspend again                   |
| **Users: DELETE** | 1     | Delete suspends the user (Coder does not hard-delete)                           |

After the scimverify suite finishes, `run.sh` does one extra curl-based check
that the suite itself does not cover: re-`POST`ing the deleted user should
reactivate the existing row (same ID) instead of creating a duplicate or
returning 409 Conflict. This exercises Coder's suspend-equals-delete semantics.

### PUT, PATCH, and DELETE

The `run.sh` script works around scimverify's `id: AUTO` limitation (which
resolves to the first user, typically the admin) by pre-creating a sacrificial
test user via `POST /Users`, capturing its UUID, and generating a temporary
config with the real ID hardcoded for PUT, PATCH, and DELETE tests. The test
user has a random name like `scimverify-a1b2c3d4` and is deleted by the
DELETE test at the end of the run.

## Configuration

All configuration lives in the heredoc inside `run.sh` (search for
`cat >"${CONFIG_FILE}"`). Always edit and run through `run.sh`; do not
invoke `npx scimverify` directly, because PUT/PATCH/DELETE rely on the
test-user UUID that `run.sh` creates and injects at runtime.

Key settings inside that heredoc:

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
  put_tests:                                    # ${TEST_USER_ID} interpolated
    - id: "${TEST_USER_ID}"
      request: { ... }
  patch_tests:                                  # ${TEST_USER_ID} interpolated
    - id: "${TEST_USER_ID}"
      request: { ... }
  delete_tests:
    - id: "${TEST_USER_ID}"

groups:
  enabled: false   # Coder does not support SCIM groups
```

### Adding test users

To add more POST test cases, append entries to the `post_tests` list in
that heredoc. Each entry needs a valid SCIM User payload. Important
Coder-specific notes:

- `userName` must be a valid Coder username (lowercase, no `@`, no spaces).
  If invalid, Coder normalizes it (e.g., `user@example.com` becomes `user`).
- `emails` must include at least one entry with `"primary": true`.
- `active` defaults to `true` if omitted.

## Expected results

On a correctly functioning Coder instance with the SCIM 2.0 refactor:

```text
# tests 22
# pass 22
# fail 0
# skipped 0
```

The `run.sh` recreate-after-delete check then prints
`ok - POST after DELETE reactivated existing user (<id>)`.
