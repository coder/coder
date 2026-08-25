---
name: add-telemetry
description: End-to-end workflow for adding a new telemetry event or summary to Coder, covering the codersdk type, the coderd reporting path, the BigQuery ingest side in coder/coder-telemetry-server, and the stacked-PR mechanics the two repos require. Use when asked to instrument a feature, measure a funnel, or ship a new field to the telemetry pipeline.
---

# Add Telemetry Skill

Instrument a feature so its data reaches BigQuery. Telemetry spans two
repositories, and a change is only finished when both sides agree on the
payload. Reviewers reject the common shortcuts, so decide the data model
before writing code.

## When to use

- A new event, counter, or summary should reach the telemetry pipeline.
- An existing telemetry struct gains a field.
- A funnel or conversion needs measuring across browser and server.

## The two sides

| Repo | Role |
|---|---|
| `coder/coder` | Defines the payload, decides who may report it, ships it in a snapshot |
| `coder/coder-telemetry-server` | Maps the payload to a BigQuery table and inserts it |

The deployment posts a `telemetry.Snapshot` as JSON. The server decodes it
into the `telemetry.Snapshot` of its pinned `coder/coder` version, so **a
field the pinned version does not know about is silently dropped**. Both sides
must land, in order, before any data appears.

## Step 1: decide the data model first

Answer these before editing, because each one is a review conversation:

- **Ephemeral event or database-backed summary?** Events reported inline via
  `api.Telemetry.Report()` need no migration. Aggregates computed on a timer
  belong with the other summary snapshots. Follow the closest existing
  pattern rather than inventing a third.
- **What identifies a row?** Use a UUID minted at the source. If two events
  must be correlated later, have the first event's ID double as the second
  event's attribution ID so the join is a self join.
- **Fixed enum or free string?** Prefer a fixed enum. Never report URLs,
  paths, organization names, template names, or anything else that can carry
  customer identifiers.
- **One table or several?** One table with an `event_type` column keeps
  funnel queries to a single self join. Separate tables force unions.
- **Who reports it?** Anything a user could forge must be reported by the
  server at the moment the real thing happens, not by the browser.
- **What is the volume?** One row per user action is fine. One row per page
  render is noise. Impression events get cut in review; ask whether the
  click alone answers the question.

## Step 2: coder/coder

1. **`codersdk/<feature>.go`**: request type plus any enums, with `Valid()`
   methods. Expose only values a client may legitimately send. If the server
   stamps a value, keep that value out of the public enum entirely, otherwise
   the generated Swagger and TypeScript union advertise something the API
   rejects.
2. **`coderd/<feature>.go`**: the handler. Authorize explicitly, validate
   every enum with its `Valid()` method, return
   `codersdk.ValidationError` per bad field, then call
   `api.Telemetry.Report()` with the telemetry struct. Stamp server-owned
   fields such as the event type and the user ID here.
3. **`coderd/coderd.go`**: register the route.
4. **`coderd/telemetry/telemetry.go`**: add the field to `Snapshot` and
   define the struct. Every field needs a `json` tag. Put server-owned
   constants next to the struct that uses them, not in `codersdk`.
5. **`enterprise/coderd/...`**: if a conversion or licensed action completes
   there, report it after the action actually succeeds.
6. **Generate**: `make gen`, or the narrower
   `make site/src/api/typesGenerated.ts coderd/apidoc/swagger.json` while
   iterating. Commit the result. `make gen` must produce no diff before
   handoff.

### Swagger annotations

Public endpoints need the full annotation block. `@ID` must be the slug of
`@Summary`, or `TestEndpointsDocumented` and
`TestEnterpriseEndpointsDocumented` fail with `Router ID must match summary`.
Editing one without the other is the single most common CI failure here.

### Go tests

Drive the endpoint through the `codersdk` client with a fake telemetry
reporter, and drain snapshots until one carries the payload, because user
creation reports its own snapshots on the same channel:

```go
func receiveEvent(ctx context.Context, t *testing.T, r *fakeTelemetryReporter) telemetry.MyEvent {
	t.Helper()
	for {
		snapshot := testutil.TryReceive(ctx, t, r.snapshots)
		if len(snapshot.MyEvents) > 0 {
			require.Len(t, snapshot.MyEvents, 1)
			return snapshot.MyEvents[0]
		}
	}
}
```

Cover the happy path, an invalid enum value, and a user without permission.

## Step 3: the browser, when the event starts there

- Report through a React Query mutation factory in `site/src/api/queries/`.
  Do not call `API.*` from a component.
- A custom hook that only wraps `useMutation` will be flagged. Call
  `useMutation(reportThing())` at the call site and keep shared logic in a
  plain function.
- Use `generateUUID()` from `#/utils/random`. `crypto.randomUUID()` is
  undefined outside secure contexts.
- Do not put `useAuthenticated()` inside a component that presentational page
  views render. Their stories mount without an auth provider, and every one
  of them starts throwing. Gate on a prop the page already computes, and let
  the API authorize the request.
- Attribution that must survive navigation belongs in `sessionStorage`, not a
  query parameter (ugly URLs) and not router state (lost on refresh). Give it
  a TTL so an abandoned token cannot attribute an unrelated later action.
- Validate anything read back out of storage with a Yup schema, and validate
  enum fields against the generated `XxxS` array from `typesGenerated.ts`.
  Hand-rolled type guards get flagged.
- Keep presentational components in `components/` free of reporting. Add an
  `onCTAClick`-style callback and own the reporting in a `modules/` wrapper,
  since `components/` must not import from `modules/`.
- Stories must click the thing and assert the reported payload. Run the
  `frontend-review` skill before pushing.

## Step 4: coder/coder-telemetry-server

1. **`convert.go`**: add a `bq<Thing>` struct. Every field needs a
   `bigquery` tag. Start with `DeploymentID` and `LoadedAt`, then mirror the
   telemetry struct. UUIDs and times become `string` and `time.Time`.
2. **`telemetry.go`**: create the table in `New()`, add the field to the
   `server` struct, and add one `convertAndInsert` call in `postSnapshot`.
3. **`convert_test.go`**: a conversion test per event type.
4. **Bump the pin**: `go get github.com/coder/coder/v2@<sha>` then
   `go mod tidy`. While the `coder/coder` side is unmerged, pin its commit
   and say so in the PR body; repoint to `main` before merging.

### The field mapping rule

`convertAndInsert` reflects over the telemetry struct's `json` tags and
requires each one to have either a matching `bigquery` column or an `ignore`
tag on the BigQuery struct. A missing mapping is not a compile error, it is a
**500 on every snapshot carrying that entity**. Extra BigQuery columns with
no telemetry counterpart are harmless.

So bumping a stale pin can break ingest for unrelated entities. Expect two
kinds of drift:

- Upstream added a field: add a column, or an `ignore` tag until the live
  table has one.
- Upstream removed a field: the tests that set or assert it stop compiling.

Verify with `go test ./...`; `TestServer/Snapshot` is what catches the 500.

**Put drift fixes in their own PR.** They are unrelated to the feature, and
burying them under a `go.sum` diff gets asked about every time. Stack the
feature on top.

## Step 5: verify

`coder/coder`:

```bash
make gen                      # must produce no diff
go build ./coderd/... ./codersdk/... ./enterprise/...
go test ./coderd/ -run TestMyFeature
go test ./coderd/coderdtest/ -run TestEndpointsDocumented
go test ./enterprise/coderd/coderdenttest/ -run TestEnterpriseEndpointsDocumented
golangci-lint run ./coderd/ ./codersdk/ ./enterprise/coderd/
cd site && pnpm check && pnpm lint:types && pnpm exec knip
cd site && pnpm exec vitest run --project=storybook src/path/to/feature
```

`coder-telemetry-server`:

```bash
go build ./... && go test ./... && gofmt -l .
```

Two traps worth knowing:

- The `typos` CI job rejects words the Go and TypeScript linters accept. It
  caught a doc comment in this very feature over a variant spelling of
  "unparsable". Run `typos --config .github/workflows/typos.toml` locally.
- Before blaming your branch for a failing story, reproduce it on `main`.
  Check out `origin/main` in the same checkout and run the same file; a
  worktree with symlinked `node_modules` fails to collect tests and proves
  nothing.

## Step 6: PR mechanics

The ingest PR cannot compile until the `coder/coder` PR merges, so the two
are always stacked. Order them `main <- drift fixes <- feature`, and state
the merge order in each body.

**Delete the base branch when you merge a stack.** GitHub retargets a
stacked PR to `main` only when its base branch is deleted. Merging the base
and then merging the child moments later lands the child on the base
*branch* instead of `main`, with green checks and no warning. Re-landing
means cherry-picking onto `main` and opening a fresh PR.

After the feature merges, repoint the ingest pin from the PR commit to
`main`.

## Step 7: prove it works

Telemetry is invisible in the UI, so the PR needs evidence:

- The exact request body and its status code.
- Any stored correlation token, showing the IDs match.
- The query the data is meant to answer, so a reviewer can judge the schema:

```sql
SELECT c.source, COUNT(*) AS clicks, COUNTIF(s.id IS NOT NULL) AS conversions
FROM my_events c
LEFT JOIN my_events s
  ON s.attribution_id = c.id AND s.event_type = 'converted'
WHERE c.event_type = 'started'
GROUP BY c.source;
```

For UI-triggered events, capture the network panel and the stored value from
a local `./scripts/develop.sh` run. See the `dogfood` skill.

## Review findings to pre-empt

These all came up in review of a real telemetry change:

- A server-only value listed in a public `codersdk` enum.
- `crypto.randomUUID()` instead of the existing helper.
- A duplicated helper copied across a stale stack instead of rebasing.
- A custom hook wrapping a single `useMutation`.
- A hand-rolled validator where Yup was already the house tool.
- An auth-context read pushed into presentational views, breaking stories.
- Dependency drift fixes mixed into a feature PR.
- An impression event nobody wanted the noise of.
