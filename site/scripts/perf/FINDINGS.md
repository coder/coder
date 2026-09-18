# Chat render performance: cause, fix, evidence

All numbers from real browsers (Chromium and WebKit, the latter being Safari's
engine), production bundle, no Storybook, no coderd, no provider. Self-contained
harness on port 8099. `scripts/perf/webkit-env.sh` makes Playwright's WebKit
runnable on this Ubuntu 26.04 image.

## Reproduction

```sh
cd site
BENCH_BUILD=1 NODE_ENV=production npx vite build
PERF_WEB_PORT=8099 PERF_WEB_ROOT=$PWD/out node scripts/perf/chat-server.mjs &

# Mount cost, before/after, both engines (medians of 3 trials)
./scripts/perf/webkit-env.sh env node scripts/perf/summary-final.mjs

# Per-row cost model in both engines
./scripts/perf/webkit-env.sh env node scripts/perf/probe-model.mjs

# Is useMemo needed at all, under React Compiler?
./scripts/perf/webkit-env.sh env node scripts/perf/probe-usememo.mjs

# Typing latency and main-thread starvation while streaming
./scripts/perf/webkit-env.sh env node scripts/perf/probe-typing.mjs

# Per-row DOM shape
./scripts/perf/webkit-env.sh env node scripts/perf/probe-rowshape.mjs

# Functional check that the transcript stays usable
node scripts/perf/validate-window.mjs
```

Query params: `panels` (open chats), `turns`, `content` (rich|prose|code|tiny),
`windowRows` (row mount window), `streamChars`. `perfCv=off` renders every row
so both arms of an A/B come from one build.

## The cause

Opening a chat mounts every message in its transcript. Main-thread cost is
linear in the number of mounted rows, measured with least squares over 8 points
(`probe-model.mjs`):

| row shape | Chromium | WebKit |
|---|---|---|
| minimal (one line) | 1.06ms + 50ms | 1.81ms + 90ms |
| realistic (prose + code) | 2.66ms + 71ms | **4.89ms + 145ms** |

So a 100-turn chat (200 rows) blocks for ~550ms in Chromium and ~1040ms in
WebKit, and open chats multiply it. The 6-chat case blocked for 2782ms
(Chromium) and 5052ms (WebKit).

Cost tracks the **sum of mounted rows across panels**, not panel count: 20 rows
total gives ~254ms whether that is 1 panel x 20 or 4 panels x 5. It also tracks
nothing about content shape per se: a one-line row still costs 1.8ms in WebKit,
and a single row is only 13-20 DOM nodes.

## The fix (deployable, in the shipped bundle)

`ConversationTimeline.tsx` mounts a bounded window of the newest 30 rows.
Older rows mount on demand through a "Show N earlier messages" control, in
steps of 30. Short transcripts are unaffected. Chosen budget: 30 rows keeps a
single chat near a 250ms block in Safari.

Before/after, medians of 3 trials, `summary-final.mjs`:

| | before | after | mounted rows |
|---|---|---|---|
| Chromium 1 chat | 551ms | **136ms** | 200 -> 30 |
| Chromium 2 chats | 987ms | **214ms** | 400 -> 60 |
| Chromium 6 chats | 2782ms | 540ms | 1200 -> 180 |
| WebKit 1 chat | 1044ms | **278ms** | 200 -> 30 |
| WebKit 2 chats | 1870ms | 422ms | 400 -> 60 |
| WebKit 6 chats | 5052ms | 982ms | 1200 -> 180 |

Also landed: nothing else in application code. An earlier revision added
`useMemo` to `ChatPageContent.tsx`'s transcript pipeline, on the theory that a
stream tick rebuilt and re-parsed the whole transcript. That theory was wrong.
`probe-usememo.mjs` builds the same component with and without the memos and
measures parses during a 6KB stream: 0 in both cases. React Compiler already
guards that pipeline, so the memos were redundant and have been removed. The
counter that read 251 parses per stream was a bench-instrumentation artifact:
the compiler hoisted the counter out of the memo guards.

Removed: the earlier `content-visibility` CSS attempt. It was measured to be a
no-op in both engines and is gone rather than left in as dead weight.

## Streaming saturation (the "typing is dead slow" symptom)

Reported: with two chats streaming, focusing the composer is slow and typing
"when" takes up to 3s to appear. The mechanism is continuous main-thread
occupancy, not one big block: input handling only runs when the thread has a
gap. Measured (`probe-typing.mjs`), 2 chats, 6000-char stream:

| | worst block | thread busy | focus latency |
|---|---|---|---|
| Chromium idle | 27ms | 0% | 3ms |
| Chromium, all rows | 164ms | **91%** | 1ms |
| Chromium, windowed | 23ms | **0%** | 6ms |
| WebKit idle | 52ms | 31% | 3ms |
| WebKit, all rows | 604ms | **84%** | 47ms |
| WebKit, windowed | 142ms | 65% | 5ms |

Windowing removes the saturation entirely in Chromium (91% -> 0%) and reduces
it in WebKit (84% -> 65%). WebKit is not fully fixed: with two streams open its
thread is still busy most of the time. Keystroke-to-DOM stayed at 2-5ms in this
synthetic stream because ticks are 33ms apart and the probe types into the gaps;
the starvation ratio is the signal that predicts the reported multi-second wait,
and it is still 65% in WebKit.

## Theories tested and DISPROVEN (each with numbers)

- **`useMemo` is needed on the transcript pipeline.** Wrong: React Compiler
  already guards it. Building the component with the memos removed still shows
  0 parses during a 6KB stream (`probe-usememo.mjs`). The memos were removed.
- **Markdown re-parsing is the bottleneck.** The live block re-parsed its whole
  accumulated text per frame: 706 runs / 2.23M chars for a 6KB stream, the same
  1457-char input parsed 9x in a row. Capping publications cut markdown 12x and
  made streaming **13x slower** (380ms -> 2761ms). Not the bottleneck.
- **Code-block syntax highlighting dominates.** Appeared to cost a fixed ~1s
  WebKit stall. **That was a harness artifact**: the bench omitted the app's
  `DiffsWorkerPoolProvider`, so highlighting ran on the main thread instead of
  a Web Worker. With the real provider stack: 661ms / 132ms block, down from
  1595ms / 1088ms. Highlighting is a real cost (~35ms/block WebKit vs ~10ms
  Chromium) but not the driver. Any future bench must mount that provider.
- **`content-visibility: auto` fixes it.** No-op in both engines (WebKit
  6039ms -> 5607ms, Chromium 1778ms -> 1737ms) because React still builds and
  commits every row; only removing rows from the box tree helped
  (`display:none`: 6039ms -> 3162ms). Superseded by real windowing.
- **Plain CSS `contain`.** `contain: layout style paint` 2881ms vs 2756ms
  baseline; `contain: style` 2787ms. No effect.
- **Stylesheet cost.** Stripping the application stylesheet saved 270ms in
  WebKit and nothing in Chromium, so it is a minor contributor, not the cause.
- **Fixed shell cost.** 6 empty transcript panels cost 42ms in WebKit. Not the
  cause.
- **Throttling smooth-text publication.** Made streaming 13x slower.
- **`contain-intrinsic-size: 240px`.** 45% scroll-height error.

## Verification

- `validate-window.mjs`: newest messages visible without scrolling; "Show 30
  earlier messages" appears only for long transcripts; clicking it mounts more
  history (30 -> 60 rows, oldest moves Q86 -> Q71); short transcripts unchanged.
- `npx tsc -p . --noEmit` clean.
- `npx biome check` clean on changed files.
- 425 unit tests pass across `ChatConversation/` and `ChatPageContent`.

## Open / not claimed

- **WebKit is improved but not at target.** 1 chat 278ms (near the 250ms bar),
  2 chats 422ms, 6 chats 982ms, and streaming still leaves the thread 65% busy.
  Further work on WebKit per-row cost (4.89ms) or on streaming work-per-tick is
  needed; windowing alone does not get multi-chat Safari under budget.
- Windowing currently reveals history by explicit control, not by scrolling to
  the top. Scroll-triggered growth would be a better UX and is not implemented.
- Only transcript rows are windowed. Header, composer and sidebar are unaffected.
- The bench navigates with a fresh page load per scenario, so absolute mount
  numbers include bundle parse/eval (~390ms Chromium, ~500ms WebKit) that an
  in-app chat switch would not pay. Both A/B arms share it.
- `longtask` is Chromium-only; WebKit numbers come from an interval-delay
  watchdog reported separately. `requestAnimationFrame` does not fire in
  headless WebKit, so frame-gap metrics are Chromium-only.
