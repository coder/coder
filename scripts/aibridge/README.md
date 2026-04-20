# AI Client Compatibility Testing (POC)

Standalone test harness that validates CLI clients work correctly through
aibridge's intercept layer. Catches breakage caused by client updates
before users hit it.

## Prerequisites

- A running Coder instance with aibridge enabled
- Claude CLI installed (`claude` on PATH)
- `ANTHROPIC_BASE_URL` set to your aibridge endpoint (e.g. `http://localhost:3000/api/v2/aibridge/anthropic`)
- `ANTHROPIC_AUTH_TOKEN` set to your `CODER_SESSION_TOKEN`

## Usage

```bash
cd scripts/aibridge

# Run all tests (all clients × all prompts)
go run main.go

# Run a specific client
go run main.go --client claude

# Run a specific prompt category
go run main.go --prompt simple

# JSON output (for CI/scripting)
go run main.go --json

# Custom timeout per test (default: 2 minutes)
go run main.go --timeout 60s

# Custom config file
go run main.go --config /path/to/config.yaml
```

## Configuration

Tests are defined in `config.yaml`. Each client specifies its CLI
invocation, required env vars, and how to detect errors in output.
Prompts are shared across all clients.

Add new clients or prompts by editing the YAML — no code changes needed.

## Example Output

```text
Summary:
--------------------------------------------------
CLIENT       PROMPT         STATUS   DURATION
--------------------------------------------------
claude       simple         PASS     2.1s
claude       tool_call      FAIL     2.9s
--------------------------------------------------
Total: 1 passed, 1 failed
```
