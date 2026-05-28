# Data Protection Mode — Complete Manual Test Results

**Test Date:** 2026-05-13  
**Branch:** `feat/data-protection-mode`  
**License:** Premium (all features)  
**Config:** `--data-protection-min-group-size=1` (disables suppression for testing)

## Tier 1 — Reporting & Analytics Surfaces

These endpoints are obfuscated when `--data-protection-mode=tier-1` or `tier-2`.

| Method + Endpoint | Field Checked | OFF | TIER-1 | TIER-2 |
|---|---|---|---|---|
| `GET /api/v2/insights/user-activity` | `report.users[].username` | `testuser8` | `Protected User 1dd86398` | `Protected User 8fcc54e1` |
| `GET /api/v2/insights/user-latency` | `report.users[].username` | `testuser8` | `Protected User 1dd86398` | `Protected User 8fcc54e1` |
| `GET /api/v2/audit` | `audit_logs[].user.username` | `admin` | `Protected User 6c6ec395` | `Protected User 584b6e68` |
| `GET /api/v2/audit` | `audit_logs[].user.email` | `admin@coder.com` | *(blank)* | *(blank)* |
| `GET /api/experimental/chats/cost/users` | `users[].username` | `testuser6` | `Protected User 3bc0662c` | `Protected User be7fc8c5` |
| `GET /api/experimental/chats/cost/users` | `users[].name` | `Test User 6` | *(blank)* | *(blank)* |
| `GET /api/experimental/chats/cost/{user}/summary` | access control | returns data | 403 Forbidden | 403 Forbidden |
| `GET /api/v2/connectionlog` | `connection_logs[].workspace_owner_username` | `bartek-regular` | `Protected User d5a02182` | `Protected User 38ba0a8a` |
| `GET /api/v2/aibridge/interceptions` | `results[].initiator.username` | `admin` | `Protected User 6c6ec395` | `Protected User 584b6e68` |
| `GET /api/v2/aibridge/sessions` | `sessions[].initiator.username` | `admin` | `Protected User 6c6ec395` | `Protected User 584b6e68` |
| `GET /api/v2/aibridge/sessions/{session_id}` | `initiator.username` | `admin` | `Protected User 6c6ec395` | `Protected User 584b6e68` |

## Tier 2 — API-Level Surfaces

These endpoints are obfuscated only when `--data-protection-mode=tier-2`.

| Method + Endpoint | Field Checked | OFF | TIER-1 | TIER-2 |
|---|---|---|---|---|
| `GET /api/v2/users` | `users[].username` (other) | `bartek-auditor` | `bartek-auditor` | `protected-6db72dfa` |
| `GET /api/v2/users` | `users[].email` (other) | `bartek@gatzbyits.com` | `bartek@gatzbyits.com` | *(blank)* |
| `GET /api/v2/users` | `users[].username` (self) | `admin` | `admin` | `admin` |
| `GET /api/v2/users/{user}` | `username` (self=me) | `admin` | `admin` | `admin` |
| `GET /api/v2/users/{user}` | `email` (self=me) | `admin@coder.com` | `admin@coder.com` | `admin@coder.com` |
| `GET /api/v2/workspaces` | `workspaces[].owner_name` | `bartek-regular` | `bartek-regular` | `protected-38ba0a8a` |
| `GET /api/v2/workspaces/{workspace}` | `owner_name` | `bartek-regular` | `bartek-regular` | `protected-38ba0a8a` |
| `GET /api/v2/@{user}/{workspace_name}` | `owner_name` | `bartek-regular` | `bartek-regular` | `protected-38ba0a8a` |
| `GET /api/v2/templates` | `[].created_by_name` (self) | `admin` | `admin` | `admin` |
| `GET /api/v2/templates/{template}` | `created_by_name` (self) | `admin` | `admin` | `admin` |
| `GET /api/v2/workspaces/{ws}/builds` | `[].initiator_name` | `bartek-regular` | `bartek-regular` | `Protected User 38ba0a8a` |
| `GET /api/v2/workspaces/{ws}/builds` | `[].workspace_owner_name` | `bartek-regular` | `bartek-regular` | `protected-38ba0a8a` |
| `GET /api/v2/workspacebuilds/{build}` | `initiator_name` | `bartek-regular` | `bartek-regular` | `Protected User 38ba0a8a` |
| `GET /api/v2/workspacebuilds/{build}` | `workspace_owner_name` | `bartek-regular` | `bartek-regular` | `protected-38ba0a8a` |
| `GET /api/v2/workspaces/{ws}/builds/{number}` | `initiator_name` | `bartek-regular` | `bartek-regular` | `Protected User 38ba0a8a` |
| `GET /api/v2/organizations/{org}/members` | `[].username` (other) | `member` | `member` | `protected-4aa97ef6` |
| `GET /api/v2/organizations/{org}/members` | `[].email` (other) | `member@coder.com` | `member@coder.com` | *(blank)* |
| `GET /api/v2/organizations/{org}/members/{user}` | `username` (CRUD resolve) | `member` | `member` | `member` |
| `GET /api/v2/organizations/{org}/paginated-members` | `members[].username` (other) | `member` | `member` | `protected-4aa97ef6` |
| `GET /api/v2/organizations/{org}/groups` | `[].members[].username` (other) | `member` | `member` | `protected-4aa97ef6` |
| `GET /api/v2/groups/{group}` | `members[].username` (other) | `member` | `member` | `protected-4aa97ef6` |

## Non-API: Prometheus Metrics

| Metric | Label | OFF | TIER-1 | TIER-2 |
|---|---|---|---|---|
| `coderd_workspace_latest_build_status` | `workspace_owner` | `bartek-regular` | `bartek-regular` | `protected-XXXXXXXX` |

*(Prometheus obfuscation is Tier 2 only. Verified in code at `coderd/prometheusmetrics/prometheusmetrics.go:191-192`. Not testable via API — requires scraping the `/metrics` endpoint.)*

## Behavior Summary

| Behavior | Verified |
|---|---|
| **Tier boundaries** | Tier-1 obfuscates only reporting surfaces. Tier-2 adds API surfaces. |
| **Self-exception** | Requesting user always sees their own identity unobfuscated (`users/me`, own workspaces, own templates). |
| **CRUD exception** | `GET /organizations/{org}/members/{user}` resolves pseudonym slug back to real user for admin operations. |
| **Pseudonym format (URL-safe)** | `protected-XXXXXXXX` for fields used as URL path params (`owner_name`, `username` in members). |
| **Pseudonym format (display)** | `Protected User XXXXXXXX` for display-only fields (`initiator_name`, audit `username`). |
| **Cross-endpoint consistency** | Same user produces same pseudonym hash within a server session across all endpoints. |
| **Pseudonym rotation** | Different HMAC keys per server restart → different pseudonyms each run. |
| **Email stripping** | Email fields blanked in obfuscated responses. |
| **Name stripping** | Name fields blanked in obfuscated responses. |
| **Access control** | `chats/cost/{user}/summary` returns 403 for non-auditors viewing others (not obfuscation — access denial). |
