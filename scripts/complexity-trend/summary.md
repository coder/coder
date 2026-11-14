# Go complexity trend: summary

Window: 2025-11-14T06:57:23Z to 2026-11-14T00:00:24Z, 6326 PRs merged into `main`. Generated Go files are excluded.

| Metric | Start | End | Change | From PRs | Not attributed to a PR |
|---|---|---|---|---|---|
| Cognitive | 44,373 | 98,147 | +53,774 (121.2%) | 53,703 | 71 |
| Cyclomatic | 37,472 | 87,723 | +50,251 (134.1%) | 50,179 | 72 |

Cognitive complexity flows over the window:

- Added by new functions: 63,172 (79% of all additions)
- Added to existing functions: 16,482
- Removed from existing functions: -8,499
- Removed with deleted functions: -17,452
- PRs that raised complexity: 1,775; lowered it: 293

Function changes: changed: 5,264, moved_from: 235, moved_to: 235, new: 19,075, removed: 4,388, touched: 16,238

## Top file hotspots

| # | File | Cognitive | Cyclomatic | PRs | Lines changed | Score |
|---|---|---|---|---|---|---|
| 1 | `coderd/exp_chats.go` | 1680 | 1509 | 264 | 26184 | 4,417,019 |
| 2 | `coderd/database/dbauthz/dbauthz.go` | 1384 | 2187 | 321 | 8139 | 2,237,042 |
| 3 | `coderd/x/chatd/chatd.go` | 385 | 400 | 336 | 34899 | 1,318,369 |
| 4 | `cli/server.go` | 715 | 485 | 86 | 1986 | 295,491 |
| 5 | `coderd/provisionerdserver/provisionerdserver.go` | 964 | 598 | 48 | 1897 | 290,892 |
| 6 | `coderd/x/chatd/chatloop/chatloop.go` | 355 | 320 | 88 | 7495 | 288,307 |
| 7 | `coderd/x/chatd/turn_environment.go` | 496 | 416 | 77 | 4161 | 280,754 |
| 8 | `codersdk/deployment.go` | 259 | 289 | 173 | 3834 | 210,935 |
| 9 | `coderd/database/db2sdk/db2sdk.go` | 352 | 402 | 113 | 3060 | 206,987 |
| 10 | `coderd/x/chatd/chatprompt/chatprompt.go` | 434 | 336 | 47 | 4667 | 203,263 |
| 11 | `codersdk/chats.go` | 130 | 194 | 219 | 9649 | 188,976 |
| 12 | `agent/agent.go` | 508 | 327 | 75 | 1674 | 180,000 |
| 13 | `coderd/x/chatd/generation.go` | 345 | 309 | 64 | 3883 | 171,986 |
| 14 | `coderd/workspaceagents.go` | 452 | 351 | 42 | 3103 | 163,175 |
| 15 | `coderd/coderd.go` | 214 | 166 | 218 | 2379 | 154,113 |

## Top function hotspots

| # | Function | Cognitive | Cyclomatic | PRs | Lines changed | Score |
|---|---|---|---|---|---|---|
| 1 | `coderd/coderd.go:New` | 171 | 120 | 200 | 2012 | 108,474 |
| 2 | `cli/server.go:RootCmd.Server` | 332 | 148 | 57 | 711 | 66,836 |
| 3 | `provisioner/terraform/resources.go:ConvertState` | 342 | 149 | 12 | 1220 | 41,381 |
| 4 | `aibridge/intercept/messages/streaming.go:StreamingInterception.ProcessRequest` | 268 | 89 | 19 | 772 | 32,458 |
| 5 | `scripts/releaser/release.go:runRelease` | 258 | 147 | 11 | 1234 | 30,059 |
| 6 | `enterprise/coderd/license/license.go:LicensesEntitlements` | 177 | 91 | 18 | 533 | 17,337 |
| 7 | `coderd/x/chatd/turn_environment.go:Server.buildTurnEnvironment` | 171 | 112 | 18 | 426 | 14,974 |
| 8 | `coderd/x/chatd/chat_mutations.go:Server.SendMessage` | 72 | 40 | 46 | 904 | 14,682 |
| 9 | `cli/ssh.go:RootCmd.ssh` | 295 | 115 | 10 | 205 | 13,357 |
| 10 | `coderd/provisionerdserver/provisionerdserver.go:server.completeWorkspaceBuildJob` | 168 | 71 | 16 | 374 | 12,996 |
| 11 | `coderd/x/chatd/chattest/openai.go:writeResponsesAPIStreaming` | 171 | 59 | 9 | 614 | 12,712 |
| 12 | `coderd/x/chatd/chattool/createworkspace.go:CreateWorkspace` | 75 | 35 | 27 | 804 | 11,050 |
| 13 | `coderd/x/chatd/chatd.go:Server.CreateChat` | 45 | 36 | 59 | 839 | 10,012 |
| 14 | `coderd/x/chatd/subagent.go:Server.subagentTools` | 47 | 26 | 28 | 1497 | 9,622 |
| 15 | `coderd/autobuild/lifecycle_executor.go:Executor.runOnce` | 219 | 50 | 11 | 162 | 9,245 |

## Net cognitive complexity growth by package (production code)

| Package directory | Net change |
|---|---|
| `coderd/x/chatd` | +4,580 |
| `coderd` | +3,723 |
| `cli` | +1,710 |
| `coderd/x/chatd/chattool` | +1,364 |
| `codersdk` | +865 |
| `coderd/x/chatd/chatloop` | +589 |
| `coderd/database/dbauthz` | +568 |
| `aibridge/intercept/messages` | +562 |
| `coderd/x/chatd/chatstate` | +534 |
| `agent/agentfiles` | +494 |
| `coderd/x/chatd/chatprompt` | +458 |
| `scripts/releaser` | +455 |
| `coderd/x/chatd/chatdebug` | +414 |
| `provisioner/terraform/scriptorder` | +414 |
| `scripts/develop` | +412 |
