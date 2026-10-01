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

const lastRequest: TypesGen.ChatMessage = {
	...MockChatMessage,
	role: "assistant",
	created_at: LAST_REQUEST_AT,
	usage: {
		input_tokens: 10,
		cache_creation_tokens: 40_000,
		cache_read_tokens: 2_000,
		context_limit: 200_000,
	},
};

const getHeat = () => {
	const heat = getChatHeat([lastRequest], 70);
	if (!heat) {
		throw new Error("expected chat heat");
	}
	return heat;
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
		vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
		vi.setSystemTime(Date.parse(LAST_REQUEST_AT) + 60_000);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("flags the cache as expired after the idle lifetime", () => {
		renderComponent(live(getHeat(), false));
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);

		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		expect(button).toHaveAccessibleName(/cache likely expired/i);
	});

	it("clears the expired flag once the chat streams again", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(live(heat, false));
		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).toHaveAccessibleName(/cache likely expired/i);

		rerender(live(heat, true));
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);
	});

	it("stays clear after a turn that ends without a counted request", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(live(heat, false));
		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).toHaveAccessibleName(/cache likely expired/i);

		rerender(live(heat, true));
		rerender(live(heat, false));
		act(() => {
			vi.advanceTimersByTime(60_000);
		});
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);
	});

	it("flags a selected model that differs from the last request's", () => {
		const heat = { ...getHeat(), lastModelConfigId: "model-a" };
		const { rerender } = renderComponent(live(heat, false, "model-a"));
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).not.toHaveAccessibleName(/model changed/i);

		rerender(live(heat, false, "model-b"));
		expect(button).toHaveAccessibleName(/model changed/i);

		// Generating with the new model is not a pending switch.
		rerender(live(heat, true, "model-b"));
		expect(button).not.toHaveAccessibleName(/model changed/i);
	});

	it("reports expiry rather than the model switch once both apply", () => {
		const heat = { ...getHeat(), lastModelConfigId: "model-a" };
		renderComponent(live(heat, false, "model-b"));
		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		expect(
			screen.getByRole("button", { name: /cache misses/i }),
		).toHaveAccessibleName(/cache likely expired\.$/i);
	});
});

describe("ChatHeatIndicator", () => {
	const renderIndicator = (
		heat: ChatHeat,
		state: { isCacheExpired?: boolean; isModelChanged?: boolean } = {},
	) =>
		renderComponent(
			<ChatHeatIndicator
				heat={heat}
				isCacheExpired={state.isCacheExpired ?? false}
				isModelChanged={state.isModelChanged ?? false}
				canSwitchModelBack
			/>,
		);
	const openTooltip = async (
		heat: ChatHeat,
		state: { isCacheExpired?: boolean; isModelChanged?: boolean } = {},
	) => {
		renderIndicator(heat, state);
		await userEvent.hover(
			screen.getByRole("button", { name: /cache misses/i }),
		);
		return screen.findByRole("tooltip");
	};

	it("names the level and score", () => {
		renderIndicator({ ...getHeat(), heat: 0.82, label: "high" });
		expect(
			screen.getByRole("button", { name: /cache misses/i }),
		).toHaveAccessibleName("Cache misses: High (82%).");
	});

	it("suggests replying sooner or compacting when high", async () => {
		const tooltip = await openTooltip({ ...getHeat(), label: "high" });
		expect(tooltip).toHaveTextContent(/reply within 5 minutes.*or compact/i);
	});

	it("explains the rebuild once the cache expired", async () => {
		const tooltip = await openTooltip(getHeat(), { isCacheExpired: true });
		expect(tooltip).toHaveTextContent(
			/next message rebuilds the cache.*reply within 5 minutes.*or compact/i,
		);
	});

	it("shows no action when not high", async () => {
		const tooltip = await openTooltip(getHeat());
		expect(tooltip).not.toHaveTextContent(/reply within|rebuilds/i);
	});

	it("states re-sent tokens against the prompt when they exceed it", async () => {
		const tooltip = await openTooltip({
			...getHeat(),
			lastTurnRequestCount: 3,
			lastTurnMissedTokens: 150_000,
			lastTurnReusableTokens: 60_000,
		});
		expect(tooltip).toHaveTextContent(
			/150K tokens against a 60K cacheable prompt/i,
		);
	});

	it("reports no reading while the latest turn is partly loaded", () => {
		renderIndicator({ ...getHeat(), lastTurnIsPartial: true });
		expect(
			screen.getByRole("button", { name: /cache misses/i }),
		).toHaveAccessibleName(
			"Cache misses: unknown. Older messages are not loaded.",
		);
	});

	it("names a changed model and its consequence", () => {
		renderIndicator(getHeat(), { isModelChanged: true });
		expect(
			screen.getByRole("button", { name: /cache misses/i }),
		).toHaveAccessibleName(/model changed; the next message will not use/i);
	});

	it("offers switching back only while the previous model is offered", async () => {
		const { unmount } = renderComponent(
			<ChatHeatIndicator
				heat={getHeat()}
				isCacheExpired={false}
				isModelChanged
				canSwitchModelBack={false}
			/>,
		);
		await userEvent.hover(
			screen.getByRole("button", { name: /cache misses/i }),
		);
		expect(await screen.findByRole("tooltip")).not.toHaveTextContent(
			/switch back/i,
		);
		unmount();

		const tooltip = await openTooltip(getHeat(), { isModelChanged: true });
		expect(tooltip).toHaveTextContent(/switch back to the previous model/i);
	});
});
