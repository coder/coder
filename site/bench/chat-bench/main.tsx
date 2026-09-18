// Self-contained benchmark entry for chat rendering.
//
// This renders the REAL chat transcript components (ChatPageTimeline ->
// ConversationTimeline -> ChatMessageItem -> Response/streamdown) against a
// real ChatStore driven by a synthetic stream producer. No network, no
// coderd, no Storybook: the only inputs are the real components and real
// store code paths, so the measured main-thread work is representative.
//
// Metrics are exposed on window.__bench for the Playwright driver:
//   - frame gaps (rAF deltas) : user-visible stutter
//   - long tasks              : main-thread blocking >= 50ms
//   - phase timestamps        : mount vs streaming attribution
import { QueryClient, QueryClientProvider } from "react-query";
import { MessageScroller } from "@shadcn/react/message-scroller";
import { createRoot } from "react-dom/client";
import { useEffect, useMemo, useRef } from "react";
import type * as TypesGen from "#/api/typesGenerated";
import { TooltipProvider } from "#/components/Tooltip/Tooltip";
import { DiffsWorkerPoolProvider } from "#/contexts/DiffsWorkerPoolProvider";
import { ThemeProvider } from "#/contexts/ThemeProvider";
import { ChatPageTimeline } from "#/pages/AgentsPage/components/ChatPageContent";
import { createChatStore } from "#/pages/AgentsPage/components/ChatConversation/chatStore";
import type { ChatStore } from "#/pages/AgentsPage/components/ChatConversation/chatStore";
// The real application stylesheet: design tokens, Tailwind layers and the
// markdown/prose rules that the transcript depends on.
import "#/index.css";

type Params = {
	/** Durable transcript turns. One turn = user + assistant message. */
	turns: number;
	/** Characters of live assistant output to stream in. 0 = no stream. */
	streamChars: number;
	/** How many independent chat panels to render at once. */
	panels: number;
	/** Stream tick interval in ms, matching real WebSocket cadence. */
	tickMs: number;
	/** Chars appended per tick. */
	charsPerTick: number;
	/**
	 * Assistant content shape. Controls which sub-systems each message
	 * exercises, so cost can be attributed to markdown vs code blocks.
	 *   rich  - prose + bullets + fenced code + table (default)
	 *   prose - prose and bullets only, no fenced code or tables
	 *   code  - fenced code only
	 *   tiny  - one short line per message, same row count, minimal text
	 */
	content: "rich" | "prose" | "code" | "tiny";
	/**
	 * Caps how many of the newest messages the store holds. At the full
	 * `turns` value nothing is dropped; lower values simulate only rendering a
	 * window of the transcript, which isolates cost per mounted row from total
	 * transcript size.
	 */
	maxRows: number;
	/** Transcript row mount window size. 0 uses the component default. */
	windowRows: number;
};

const readParams = (): Params => {
	const q = new URLSearchParams(location.search);
	const num = (key: string, fallback: number) => {
		const raw = q.get(key);
		if (raw === null) return fallback;
		const parsed = Number(raw);
		return Number.isFinite(parsed) ? parsed : fallback;
	};
	return {
		turns: num("turns", 25),
		streamChars: num("streamChars", 0),
		panels: num("panels", 1),
		tickMs: num("tickMs", 33),
		charsPerTick: num("charsPerTick", 24),
		content: (q.get("content") ?? "rich") as Params["content"],
		maxRows: num("maxRows", 0),
		windowRows: num("windowRows", 0),
	};
};

const baseMessage = {
	chat_id: "bench-chat",
	created_at: "2026-03-10T00:00:00.000Z",
} as const;

// Markdown shaped like real agent output: prose, bullet list, fenced code,
// and a table, so streamdown/remark/micromark do realistic work.
const buildAssistantText = (
	turn: number,
	shape: Params["content"],
): string => {
	if (shape === "tiny") {
		// Same row count as any other shape, minimal text: separates cost per
		// mounted row from cost per character of text.
		return `Answer ${turn} ok.`;
	}
	const lines: string[] = [];
	lines.push(`## Analysis for concept ${turn}`);
	lines.push("");
	lines.push(
		`Here is a detailed explanation of concept ${turn}. `.repeat(6).trim(),
	);
	lines.push("");
	lines.push("Key points to consider:");
	lines.push("");
	for (let i = 1; i <= 5; i++) {
		lines.push(
			`- Point ${i}: the mechanism requires careful handling of edge cases and validation`,
		);
	}
	lines.push("");
	if (shape === "code") {
		lines.length = 0;
	}
	if (shape !== "prose") {
		lines.push("```typescript");
		lines.push(`function processConcept${turn}(input: string[]): number {`);
		lines.push("  let total = 0;");
		lines.push("  for (const item of input) {");
		lines.push("    total += item.length;");
		lines.push("  }");
		lines.push("  return total;");
		lines.push("}");
		lines.push("```");
		lines.push("");
		lines.push("| Field | Value | Notes |");
		lines.push("| ----- | ----- | ----- |");
		lines.push(`| concept | ${turn} | derived from input |`);
		lines.push("| status | active | current |");
		lines.push("");
	}
	lines.push(
		"Additional context follows here with more prose that explains the reasoning behind the approach and why alternatives were rejected in this scenario. ".repeat(
			3,
		),
	);
	return lines.join("\n");
};

const buildMessages = (
	turns: number,
	shape: Params["content"],
): TypesGen.ChatMessage[] => {
	const messages: TypesGen.ChatMessage[] = [];
	for (let turn = 1; turn <= turns; turn++) {
		const userId = turn * 2 - 1;
		const assistantId = turn * 2;
		messages.push({
			id: userId,
			...baseMessage,
			role: "user" as TypesGen.ChatMessageRole,
			content: [
				{
					type: "text",
					text: `Question ${turn}: Can you explain concept ${turn} in detail and show an example?`,
				},
			],
		});
		messages.push({
			id: assistantId,
			...baseMessage,
			role: "assistant" as TypesGen.ChatMessageRole,
			content: [{ type: "text", text: buildAssistantText(turn, shape) }],
		});
	}
	return messages;
};

const STREAM_PARAGRAPH =
	"The streaming response continues with more detailed analysis of the current concept and its practical implications for the workload. ";

/**
 * Produces a growing assistant text body, matching how a real provider
 * delta stream accumulates into one response block.
 */
const buildStreamText = (chars: number): string => {
	const repeats = Math.ceil(chars / STREAM_PARAGRAPH.length);
	return STREAM_PARAGRAPH.repeat(repeats).slice(0, chars);
};

type BenchedPanel = {
	store: ChatStore;
	intervalId: number | null;
	emitted: number;
	start(): void;
	stop(): void;
};

const createPanel = (turns: number, streamChars: number, params: Params) => {
	const store = createChatStore();
	const allMessages = buildMessages(turns, params.content);
	// maxRows > 0 keeps only the newest rows, simulating a windowed
	// transcript, so cost per mounted row can be separated from the size of
	// the full history.
	const messages =
		params.maxRows > 0 && params.maxRows < allMessages.length
			? allMessages.slice(-params.maxRows)
			: allMessages;
	store.replaceMessages(messages);
	store.setActiveChatID("bench-chat");

	let emitted = 0;
	let intervalId: number | null = null;
	let current = 0;

	const panel: BenchedPanel = {
		store,
		get intervalId() {
			return intervalId;
		},
		get emitted() {
			return emitted;
		},
		start() {
			if (streamChars <= 0 || intervalId !== null) return;
			// A live turn: status running plus a growing response block.
			store.setChatStatus("running");
			// Seed the first delta so the live row exists immediately.
			store.applyMessagePart({
				type: "text",
				text: STREAM_PARAGRAPH.slice(0, params.charsPerTick),
			} as TypesGen.ChatMessagePart);
			current = params.charsPerTick;
			emitted += 1;
			intervalId = window.setInterval(() => {
				current += params.charsPerTick;
				const full = buildStreamText(current);
				const delta = full.slice(current - params.charsPerTick);
				if (delta) {
					// Mirrors useChatStore's applyMessageParts path: one
					// store mutation per server event batch.
					store.applyMessageParts([
						{ type: "text", text: delta } as TypesGen.ChatMessagePart,
					]);
					emitted += 1;
				}
				if (current >= streamChars) {
					panel.stop();
					store.setChatStatus("waiting");
					(window as unknown as BenchWindow).__bench.streamDone = true;
				}
			}, params.tickMs);
		},
		stop() {
			if (intervalId !== null) {
				window.clearInterval(intervalId);
				intervalId = null;
			}
		},
	};
	return panel;
};

type BenchWindow = Window & {
	__bench: BenchState;
};

type BenchState = {
	ready: boolean;
	phaseStart: number;
	streamDone: boolean;
	frames: number[];
	intervalDelays: number[];
	longTasks: Array<{ start: number; duration: number }>;
	markPhase(name: string): void;
	summary(): BenchSummary;
	phaseSummary(name: string): PhaseSummary;
	counters(): Record<string, number>;
};

type BenchSummary = {
	frames: number;
	longTaskMs: number;
	longTaskCount: number;
	worstLongTaskMs: number;
	worstFrameGapMs: number;
	framesOver50ms: number;
	framesOver100ms: number;
	p95FrameGapMs: number;
	droppedFrameRatio: number;
	sampleWindowMs: number;
};

type PhaseSummary = {
	longTaskMs: number;
	longTaskCount: number;
	worstLongTaskMs: number;
	frames: number;
	worstFrameGapMs: number;
	framesOver50ms: number;
	durationMs: number;
	/** Longest single main-thread block, from the interval watchdog. */
	worstBlockMs: number;
	/** Total lateness of watchdog fires beyond 50ms, per phase. */
	blockedMs: number;
};

const installInstrumentation = (): void => {
	const win = window as unknown as BenchWindow;
	// Enables the app's opt-in perf counters (see useOnRenderProfiler).
	(window as unknown as Record<string, unknown>).__perfCounters = {
		parseCalls: 0,
		messagesParsed: 0,
		timelineRenders: 0,
				markdownRuns: 0,
		markdownChars: 0,
		markdownLens: [],
	};
	const phaseStarts: Record<string, number> = {};
	const phaseTaskStart: Record<string, number> = {};
	const phaseFrameStart: Record<string, number> = {};
	const phaseDelayStart: Record<string, number> = {};
	const state: BenchState = {
		ready: false,
		phaseStart: performance.now(),
		streamDone: false,
		frames: [],
		longTasks: [],
		markPhase: () => {},
		summary: () => ({
			frames: 0,
			longTaskMs: 0,
			longTaskCount: 0,
			worstLongTaskMs: 0,
			worstFrameGapMs: 0,
			framesOver50ms: 0,
			framesOver100ms: 0,
			p95FrameGapMs: 0,
			droppedFrameRatio: 0,
			sampleWindowMs: 0,
		}),
		phaseSummary: () => ({
			longTaskMs: 0,
			longTaskCount: 0,
			worstLongTaskMs: 0,
			frames: 0,
			worstFrameGapMs: 0,
			framesOver50ms: 0,
			durationMs: 0,
			worstBlockMs: 0,
			blockedMs: 0,
		}),
		counters: () => ({}),
		intervalDelays: [],
	};
	state.markPhase = (name: string) => {
		state.phaseStart = performance.now();
		(window as unknown as Record<string, number>)[`__phase_${name}`] =
			state.phaseStart;
		phaseStarts[name] = state.phaseStart;
		phaseTaskStart[name] = state.longTasks.length;
		phaseFrameStart[name] = state.frames.length;
		phaseDelayStart[name] = state.intervalDelays.length;
	};
	state.phaseSummary = (name: string): PhaseSummary => {
		const start = phaseStarts[name] ?? 0;
		const tasks = state.longTasks
			.slice(phaseTaskStart[name] ?? 0)
			.filter((task) => task.start >= start);
		const gapSlice = state.frames.slice(phaseFrameStart[name] ?? 0);
		const gaps = gapSlice.filter((gap) => gap < 1000);
		const sum = (arr: number[]) => arr.reduce((a, b) => a + b, 0);
		// Watchdog lateness for this phase: the engine-agnostic blocking signal.
		const delaySlice = state.intervalDelays.slice(phaseDelayStart[name] ?? 0);
		const delays = delaySlice.filter((delay) => delay >= 0);
		const overBudget = delays.filter((delay) => delay > 50);
		return {
			longTaskMs: Math.round(sum(tasks.map((t) => t.duration))),
			longTaskCount: tasks.length,
			worstLongTaskMs: Math.round(
				Math.max(0, ...tasks.map((t) => t.duration)),
			),
			frames: gaps.length,
			worstFrameGapMs: Math.round(Math.max(0, ...gaps) * 10) / 10,
			framesOver50ms: gaps.filter((g) => g > 50).length,
			durationMs: Math.round(performance.now() - start),
			worstBlockMs: Math.round(Math.max(0, ...delays) * 10) / 10,
			blockedMs: Math.round(sum(overBudget)),
		};
	};
	// Counters attribute CPU cost to a specific code path rather than only
	// to elapsed time.
	state.counters = () =>
		(window as unknown as { __perfCounters?: Record<string, number> })
			.__perfCounters ?? {};
	state.markPhase("mount");
	state.summary = () => {
		const gaps = state.frames.filter((gap) => gap < 1000);
		const sorted = [...gaps].sort((a, b) => a - b);
		const sum = (arr: number[]) => arr.reduce((a, b) => a + b, 0);
		return {
			frames: gaps.length,
			longTaskMs: Math.round(sum(state.longTasks.map((t) => t.duration))),
			longTaskCount: state.longTasks.length,
			worstLongTaskMs: Math.round(
				Math.max(0, ...state.longTasks.map((t) => t.duration)),
			),
			worstFrameGapMs: Math.round(Math.max(0, ...gaps) * 10) / 10,
			framesOver50ms: gaps.filter((g) => g > 50).length,
			framesOver100ms: gaps.filter((g) => g > 100).length,
			p95FrameGapMs:
				Math.round((sorted[Math.floor(sorted.length * 0.95)] ?? 0) * 10) / 10,
			droppedFrameRatio:
				Math.round(
					(gaps.filter((g) => g > 20).length / Math.max(1, gaps.length)) * 1000,
				) / 1000,
			sampleWindowMs: Math.round(
				performance.now() - (state.phaseStart || 0),
			),
		};
	};
	win.__bench = state;

	try {
		new PerformanceObserver((list) => {
			for (const entry of list.getEntries()) {
				state.longTasks.push({
					start: entry.startTime,
					duration: entry.duration,
				});
			}
		}).observe({ entryTypes: ["longtask"] });
	} catch {
		// longtask unsupported
	}

	let last = performance.now();
	const loop = () => {
		const now = performance.now();
		state.frames.push(now - last);
		last = now;
		requestAnimationFrame(loop);
	};
	requestAnimationFrame(loop);

	// Interval-delay watchdog. requestAnimationFrame does not fire in headless
	// WebKit (no compositor), so frame gaps are Chromium-only in practice.
	// A self-rescheduling interval still fires in WebKit, and the lateness of
	// each fire measures exactly the main-thread blocking that frame gaps
	// measure in Chromium. Recorded separately so neither metric pollutes the
	// other.
	const WATCHDOG_MS = 16;
	state.intervalDelays = [];
	let expected = performance.now() + WATCHDOG_MS;
	window.setInterval(() => {
		const now = performance.now();
		state.intervalDelays.push(now - expected);
		expected = now + WATCHDOG_MS;
	}, WATCHDOG_MS);
};

const BenchApp = ({ params, windowRows }: { params: Params; windowRows?: number }) => {
	const panelsRef = useRef<BenchedPanel[] | null>(null);
	if (panelsRef.current === null) {
		panelsRef.current = Array.from({ length: params.panels }, () =>
			createPanel(params.turns, params.streamChars, params),
		);
	}
	const panels = panelsRef.current;
	const queryClient = useMemo(
		() =>
			new QueryClient({
				defaultOptions: { queries: { retry: false, enabled: false } },
			}),
		[],
	);

	useEffect(() => {
		const win = window as unknown as BenchWindow;
		// Let the mount commit paint before declaring ready, so the driver
		// can attribute mount cost separately from streaming cost.
		requestAnimationFrame(() => {
			requestAnimationFrame(() => {
				win.__bench.ready = true;
				win.__bench.markPhase("stream");
				for (const panel of panels) {
					panel.start();
				}
			});
		});
		return () => {
			for (const panel of panels) {
				panel.stop();
			}
		};
	}, [panels]);

	return (
		<QueryClientProvider client={queryClient}>
			{/* DiffsWorkerPoolProvider is part of the real app's provider stack
			    (App.tsx) and moves syntax highlighting into a Web Worker. Without
			    it the highlighter runs on the main thread, which would make the
			    measurement unrepresentative of the application. */}
			<DiffsWorkerPoolProvider>
				<ThemeProvider>
				<TooltipProvider delayDuration={100}>
				<div style={{ display: "flex", height: "100vh", width: "100vw" }}>
					{panels.map((panel, index) => (
						// biome-ignore lint/suspicious/noArrayIndexKey: fixed panel count
						<div key={index} style={{ flex: 1, minWidth: 0, height: "100%" }}>
							<MessageScroller.Provider autoScroll defaultScrollPosition="end">
								<ChatPageTimeline
									organizationId="bench-org"
									store={panel.store}
									persistedError={undefined}
									hasMoreMessages={false}
									isFetchingMoreMessages={false}
									isHydratingMessages={false}
									hasFetchMoreError={false}
									onFetchMoreMessages={async () => {}}
									windowRows={windowRows}
								/>
							</MessageScroller.Provider>
						</div>
					))}
				</div>
				</TooltipProvider>
				</ThemeProvider>
			</DiffsWorkerPoolProvider>
		</QueryClientProvider>
	);
};

// Set before React mounts by the perfCv switch in start().
let windowRowsOverride: number | null = null;

const start = () => {
	installInstrumentation();
	// perfCv=off disables row windowing (renders every row) so the same build
	// can be measured with the fix on and off. perfCv=none builds every row's
	// DOM but removes it from layout and paint, which separates DOM
	// construction cost from layout cost.
	const cvMode = new URLSearchParams(location.search).get("perfCv");
	if (cvMode === "off") {
		// ∞ rows: renderRows.length is always smaller, so nothing is windowed.
		windowRowsOverride = Number.MAX_SAFE_INTEGER;
	} else if (cvMode === "none") {
		const style = document.createElement("style");
		style.textContent = "[data-message-id] { display: none !important; }";
		document.head.appendChild(style);
	}
	// Diagnostic: perfStripCss=1 removes the application stylesheet just
	// before the app mounts, keeping the DOM structure and node count
	// identical. Isolates style recalculation cost (including :has()
	// selectors) from React and layout cost.
	if (
		new URLSearchParams(location.search).get("perfStripCss") === "1"
	) {
		for (const link of document.querySelectorAll('link[rel="stylesheet"]')) {
			link.remove();
		}
		for (const style of document.querySelectorAll("style")) {
			style.remove();
		}
	}
	const params = readParams();
	// windowRowsOverride comes from the perfCv switch above; a URL windowRows
	// parameter is used when the switch is absent.
	const effectiveWindowRows =
		windowRowsOverride ?? (params.windowRows || undefined);
	const root = createRoot(document.getElementById("bench-root") as HTMLElement);
	// No StrictMode: it double-invokes render in development builds, which
	// would double every measured render. Measurements need one commit per
	// store update, matching the production behaviour being diagnosed.
	root.render(<BenchApp params={params} windowRows={effectiveWindowRows} />);
};

start();
