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

describe("LiveChatHeatIndicator", () => {
	beforeEach(() => {
		vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
		vi.setSystemTime(Date.parse(LAST_REQUEST_AT) + 60_000);
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("flags the cache as expired after the idle lifetime", () => {
		renderComponent(
			<LiveChatHeatIndicator heat={getHeat()} isStreaming={false} />,
		);
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);

		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		expect(button).toHaveAccessibleName(/cache likely expired/i);
	});

	it("clears the expired flag once the chat streams again", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(
			<LiveChatHeatIndicator heat={heat} isStreaming={false} />,
		);
		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).toHaveAccessibleName(/cache likely expired/i);

		rerender(<LiveChatHeatIndicator heat={heat} isStreaming />);
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);
	});

	it("stays clear after a turn that ends without a counted request", () => {
		const heat = getHeat();
		const { rerender } = renderComponent(
			<LiveChatHeatIndicator heat={heat} isStreaming={false} />,
		);
		act(() => {
			vi.advanceTimersByTime(CACHE_IDLE_TTL_MS);
		});
		const button = screen.getByRole("button", { name: /cache misses/i });
		expect(button).toHaveAccessibleName(/cache likely expired/i);

		rerender(<LiveChatHeatIndicator heat={heat} isStreaming />);
		rerender(<LiveChatHeatIndicator heat={heat} isStreaming={false} />);
		act(() => {
			vi.advanceTimersByTime(60_000);
		});
		expect(button).not.toHaveAccessibleName(/cache likely expired/i);
	});
});

describe("ChatHeatIndicator", () => {
	const openTooltip = async (heat: ChatHeat, isCacheExpired: boolean) => {
		renderComponent(
			<ChatHeatIndicator heat={heat} isCacheExpired={isCacheExpired} />,
		);
		await userEvent.hover(
			screen.getByRole("button", { name: /cache misses/i }),
		);
		return screen.findByRole("tooltip");
	};

	it("names the level and score", () => {
		renderComponent(
			<ChatHeatIndicator
				heat={{ ...getHeat(), heat: 0.82, label: "high" }}
				isCacheExpired={false}
			/>,
		);
		expect(
			screen.getByRole("button", { name: /cache misses/i }),
		).toHaveAccessibleName("Cache misses: High (82%).");
	});

	it("suggests replying sooner when high", async () => {
		const tooltip = await openTooltip({ ...getHeat(), label: "high" }, false);
		expect(tooltip).toHaveTextContent(/replies after 5 minutes re-send/i);
	});

	it("suggests replying or compacting once the cache expired", async () => {
		const tooltip = await openTooltip(getHeat(), true);
		expect(tooltip).toHaveTextContent(/reply within 5 minutes.*or compact/i);
	});

	it("shows no action when not high", async () => {
		const tooltip = await openTooltip(getHeat(), false);
		expect(tooltip).not.toHaveTextContent(/reply within|replies after/i);
	});
});
