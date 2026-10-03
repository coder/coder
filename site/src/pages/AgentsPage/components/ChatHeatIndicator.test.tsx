import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "#/api/typesGenerated";
import { MockChatMessage } from "#/testHelpers/chatEntities";
import { renderComponent } from "#/testHelpers/renderHelpers";
import {
	CACHE_IDLE_TTL_MS,
	type ChatHeat,
	getChatHeat,
} from "./ChatConversation/chatHeat";
import { ChatHeatIndicator, LiveChatHeatIndicator } from "./ChatHeatIndicator";

const LAST_REQUEST_AT = "2026-01-01T00:00:00Z";
const MINUTE_MS = 60_000;

const requestWith = (promptTokens: number): TypesGen.ChatMessage => ({
	...MockChatMessage,
	role: "assistant",
	created_at: LAST_REQUEST_AT,
	model_config_id: "model-a",
	usage: {
		input_tokens: 10,
		cache_creation_tokens: 2_000,
		cache_read_tokens: promptTokens - 2_010,
		context_limit: 1_000_000,
	},
});

// Two requests so the latest turn has a cacheable previous prompt.
const getHeat = (promptTokens = 165_000): ChatHeat => {
	const heat = getChatHeat([
		{ ...MockChatMessage, role: "user" },
		requestWith(promptTokens),
		requestWith(promptTokens),
	]);
	if (!heat) {
		throw new Error("expected chat heat");
	}
	return heat;
};

const compactedHeat: ChatHeat = {
	lastPromptTokens: 0,
	lastRequestAt: "",
	lastModelConfigId: undefined,
	boundary: "compacted",
	lastTurn: undefined,
};

const button = () => screen.getByRole("button", { name: /next message/i });
const badge = () => screen.queryByTestId("heat-badge");

// Timers chain through effects: each tick schedules the next from the
// clock it just read, so advance one minute per act, slightly past the
// boundary, rather than several minutes in one step.
const advanceMinutes = (minutes: number) => {
	for (let i = 0; i < minutes; i++) {
		act(() => {
			vi.advanceTimersByTime(MINUTE_MS + 100);
		});
	}
};

const live = (
	heat: ChatHeat,
	isStreaming: boolean,
	selectedModelConfigId = "model-a",
) => (
	<LiveChatHeatIndicator
		heat={heat}
		isStreaming={isStreaming}
		selectedModelConfigId={selectedModelConfigId}
		selectableModelConfigIds={["model-a", "model-b"]}
	/>
);

describe("LiveChatHeatIndicator", () => {
	beforeEach(() => {
		vi.useFakeTimers({
			toFake: ["setTimeout", "clearTimeout", "setInterval", "Date"],
		});
		vi.setSystemTime(Date.parse(LAST_REQUEST_AT) + 30_000);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("counts whole minutes down and flips to cold at expiry", () => {
		renderComponent(live(getHeat(), false));
		expect(button()).toHaveAccessibleName(
			/^Next message: Low\. Reads 165K.*expires in about 5 minutes\.$/,
		);
		expect(badge()).toHaveTextContent("5");
		expect(badge()).toHaveClass("bg-surface-primary");

		// The clock is re-read at minute boundaries, not every second.
		advanceMinutes(1);
		expect(badge()).toHaveTextContent("4");
		expect(button()).toHaveAccessibleName(/expires in about 4 minutes/i);

		advanceMinutes(2);
		expect(badge()).toHaveTextContent("2");
		expect(badge()).toHaveClass("bg-surface-primary");

		advanceMinutes(1);
		expect(badge()).toHaveTextContent("1");
		expect(badge()).toHaveClass("bg-content-primary", "animate-pulse");
		expect(button()).toHaveAccessibleName(/expires in under a minute/i);

		act(() => {
			vi.advanceTimersByTime(31_000);
		});
		expect(badge()).not.toHaveTextContent(/\d/);
		expect(badge()).not.toHaveClass("animate-pulse");
		expect(badge()).toHaveClass("bg-content-primary");
		expect(button()).toHaveAccessibleName(
			/^Next message: High\. Re-writes about 165K tokens.*Cache likely expired/,
		);
	});

	it("re-reads the clock when a timer fires late", () => {
		renderComponent(live(getHeat(), false));
		// The wall clock jumps (a suspended machine) before the pending minute
		// tick fires; the tick reads the clock rather than assuming a minute.
		vi.setSystemTime(Date.now() + 10 * MINUTE_MS);
		act(() => {
			vi.advanceTimersByTime(MINUTE_MS);
		});
		expect(button()).toHaveAccessibleName(/cache likely expired/i);
	});

	it("re-reads the clock when the tab becomes visible", () => {
		renderComponent(live(getHeat(), false));
		vi.setSystemTime(Date.now() + 2 * MINUTE_MS);
		act(() => {
			document.dispatchEvent(new Event("visibilitychange"));
		});
		expect(badge()).toHaveTextContent("3");
	});

	it("clears its timer on unmount", () => {
		const { unmount } = renderComponent(live(getHeat(), false));
		expect(vi.getTimerCount()).toBe(1);
		unmount();
		expect(vi.getTimerCount()).toBe(0);
	});

	it("shows the seconds while the tooltip is open", async () => {
		const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
		renderComponent(live(getHeat(), false));
		// 25 s into the minute, so the stored minute tick is stale.
		act(() => {
			vi.advanceTimersByTime(25_000);
		});
		await user.hover(button());
		// Past the tooltip's open delay.
		act(() => {
			vi.advanceTimersByTime(200);
		});
		const tooltip = screen.getByRole("tooltip");
		expect(tooltip).toHaveTextContent(/cache expires in 4:05\./i);

		act(() => {
			vi.advanceTimersByTime(1_000);
		});
		expect(tooltip).toHaveTextContent(/cache expires in 4:04\./i);
		// The name stays on whole minutes while focused.
		expect(button()).toHaveAccessibleName(/expires in about 5 minutes/i);
	});

	it("hides the countdown while generating and restarts it after", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(live(heat, true));
		expect(badge()).toBeNull();
		expect(button()).toHaveAccessibleName(/^Next message: Low/);

		advanceMinutes(CACHE_IDLE_TTL_MS / MINUTE_MS);
		expect(button()).not.toHaveAccessibleName(/expired/i);

		// The stream ends without a newer counted request; the lifetime runs
		// from the stream end.
		rerender(live(heat, false));
		expect(badge()).toHaveTextContent("5");
		advanceMinutes(CACHE_IDLE_TTL_MS / MINUTE_MS - 1);
		expect(badge()).toHaveTextContent("1");
	});

	it("flags a selected model that differs from the last request's", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(live(heat, false, "model-a"));
		expect(button()).not.toHaveAccessibleName(/model changed/i);

		rerender(live(heat, false, "model-b"));
		expect(button()).toHaveAccessibleName(
			/^Next message: High\. Re-writes.*Model changed/,
		);
		expect(badge()).not.toHaveTextContent(/\d/);
		expect(badge()).toHaveClass("bg-content-primary");

		// Generating with the new model is not a pending switch.
		rerender(live(heat, true, "model-b"));
		expect(button()).not.toHaveAccessibleName(/model changed/i);
	});

	it("reports expiry rather than the model switch once both apply", () => {
		renderComponent(live(getHeat(), false, "model-b"));
		advanceMinutes(CACHE_IDLE_TTL_MS / MINUTE_MS);
		expect(button()).toHaveAccessibleName(/cache likely expired\.$/i);
	});

	it("shows no countdown after a compaction", () => {
		renderComponent(live(compactedHeat, false));
		expect(badge()).toBeNull();
		expect(button()).toHaveAccessibleName(
			"Next message: fresh cache. Compacted: the next message writes a fresh cache from the summary.",
		);
	});
});

describe("ChatHeatIndicator", () => {
	type State = {
		remainingMs?: number;
		isModelChanged?: boolean;
		canSwitchModelBack?: boolean;
	};
	const renderIndicator = (heat: ChatHeat, state: State = {}) =>
		renderComponent(
			<ChatHeatIndicator
				heat={heat}
				remainingMs={state.remainingMs}
				isModelChanged={state.isModelChanged ?? false}
				canSwitchModelBack={state.canSwitchModelBack ?? true}
			/>,
		);
	const openTooltip = async (heat: ChatHeat, state: State = {}) => {
		renderIndicator(heat, state);
		await userEvent.hover(button());
		return screen.findByRole("tooltip");
	};

	it("describes a warm cache with the scaled re-write", async () => {
		const tooltip = await openTooltip(getHeat(), {
			remainingMs: 3 * MINUTE_MS + 42_000,
		});
		expect(tooltip).toHaveTextContent(
			/reads 165K tokens from the cache, about 13K tokens' worth of a re-write \(Low\)/i,
		);
		expect(tooltip).toHaveTextContent(/cache expires in 3:42/i);
		expect(tooltip).not.toHaveTextContent(/compact|reply now/i);
		expect(tooltip).toHaveTextContent(
			/last turn: 2 requests re-sent 2K tokens \(1% of 165K cacheable\)\./i,
		);
		expect(screen.getByTestId("at-stake")).toBeInTheDocument();
	});

	it("asks for a reply in the last minute", async () => {
		const tooltip = await openTooltip(getHeat(), { remainingMs: 42_000 });
		expect(tooltip).toHaveTextContent(/cache expires in 0:42\./i);
		expect(tooltip).toHaveTextContent(/reply now to keep it\./i);
	});

	it("enters the last minute at exactly sixty seconds", () => {
		renderIndicator(getHeat(), { remainingMs: MINUTE_MS });
		expect(badge()).toHaveTextContent("1");
		expect(badge()).toHaveClass("bg-content-primary", "animate-pulse");
	});

	it("suggests compacting or clearing when cold and at least moderate", async () => {
		const tooltip = await openTooltip(getHeat(), {
			remainingMs: -12 * MINUTE_MS,
		});
		expect(tooltip).toHaveTextContent(
			/re-writes about 165K tokens without the cache \(High\)/i,
		);
		expect(tooltip).toHaveTextContent(/expired 12 minutes ago/i);
		expect(tooltip).toHaveTextContent(/type \/compact .* or \/clear/i);
		expect(screen.queryByTestId("at-stake")).toBeNull();
	});

	it("skips the compact hint for a small cold prompt", async () => {
		const tooltip = await openTooltip(getHeat(50_000), {
			remainingMs: -MINUTE_MS,
		});
		expect(tooltip).toHaveTextContent(/\(Low\)/);
		expect(tooltip).not.toHaveTextContent(/compact/i);
	});

	it("gates the compact hint on the moderate band edge", async () => {
		const { unmount } = renderIndicator(getHeat(61_000), {
			remainingMs: -MINUTE_MS,
		});
		await userEvent.hover(button());
		expect(await screen.findByRole("tooltip")).not.toHaveTextContent(
			/compact/i,
		);
		unmount();

		const tooltip = await openTooltip(getHeat(63_000), {
			remainingMs: -MINUTE_MS,
		});
		expect(tooltip).toHaveTextContent(/\(Moderate\)/);
		expect(tooltip).toHaveTextContent(/type \/compact/i);
	});

	it("offers switching back with the time left while the model is offered", async () => {
		const tooltip = await openTooltip(getHeat(), {
			remainingMs: 2 * MINUTE_MS,
			isModelChanged: true,
		});
		expect(tooltip).toHaveTextContent(
			/selected model differs.*switch back within 2:00/i,
		);
	});

	it("offers switching back without a time when no countdown is known", async () => {
		const tooltip = await openTooltip(getHeat(), { isModelChanged: true });
		expect(tooltip).toHaveTextContent(
			/switch back to the previous model to reuse its cache\./i,
		);
	});

	it("falls back to the compact hint when the model cannot be switched back", async () => {
		const tooltip = await openTooltip(getHeat(), {
			remainingMs: 2 * MINUTE_MS,
			isModelChanged: true,
			canSwitchModelBack: false,
		});
		expect(tooltip).not.toHaveTextContent(/switch back/i);
		expect(tooltip).toHaveTextContent(/type \/compact/i);
	});

	it("details the last turn's misses", async () => {
		const heat = getHeat();
		const tooltip = await openTooltip({
			...heat,
			lastTurn: {
				requestCount: 3,
				missedTokens: 150_000,
				reusableTokens: 60_000,
				isPartial: false,
			},
		});
		expect(tooltip).toHaveTextContent(
			/last turn: 3 requests re-sent 150K tokens against a 60K cacheable prompt/i,
		);
	});

	it("keeps the reading when the latest turn is partly loaded", async () => {
		const heat = getHeat();
		const tooltip = await openTooltip({
			...heat,
			lastTurn: { ...heat.lastTurn!, isPartial: true },
		});
		expect(button()).toHaveAccessibleName(/^Next message: Low\. Reads 165K/);
		expect(tooltip).toHaveTextContent(
			/earlier requests in this turn are not loaded/i,
		);
	});

	it("explains a cleared context", async () => {
		const tooltip = await openTooltip({
			...compactedHeat,
			boundary: "cleared",
		});
		expect(tooltip).toHaveTextContent(/cleared: the next message writes/i);
		expect(badge()).toBeNull();
	});
});
