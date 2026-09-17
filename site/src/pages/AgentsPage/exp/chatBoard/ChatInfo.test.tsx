import { act, fireEvent, screen } from "@testing-library/react";
import { QueryClientProvider } from "react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { API } from "#/api/api";
import { chat as chatQuery } from "#/api/queries/chats";
import { MockChat } from "#/testHelpers/chatEntities";
import {
	createTestQueryClient,
	renderComponent,
} from "#/testHelpers/renderHelpers";
import { ChatInfoPopover } from "./ChatInfo";

vi.mock("#/modules/dashboard/useFeatureVisibility", () => ({
	useFeatureVisibility: () => ({ aibridge: false }),
}));

// The popover body fetches the chat detail only while it is open, so the
// detail request is the observable for opening, and a refetch on
// invalidation is the observable for staying open.
const renderInfo = () => {
	const queryClient = createTestQueryClient();
	renderComponent(
		<QueryClientProvider client={queryClient}>
			<ChatInfoPopover chat={MockChat} />
		</QueryClientProvider>,
	);
	const trigger = screen.getByRole("button", {
		name: `Details for ${MockChat.title}`,
	});
	const invalidate = async () => {
		// Settle the fetch in flight first, so invalidation refetches rather
		// than joining it.
		await act(async () => {});
		await act(() =>
			queryClient.invalidateQueries({
				queryKey: chatQuery(MockChat.id).queryKey,
			}),
		);
	};
	return { trigger, invalidate };
};

// Synchronous events, so each state change and its effect commit before
// the clock moves.
const wait = (ms: number) => act(() => vi.advanceTimersByTime(ms));

describe("ChatInfoPopover", () => {
	beforeEach(() => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
		vi.spyOn(API.experimental, "getChat").mockResolvedValue(MockChat);
	});
	afterEach(() => {
		vi.useRealTimers();
		vi.restoreAllMocks();
	});

	it("fetches the detail after resting on the icon, and once more only while open", async () => {
		const { trigger, invalidate } = renderInfo();

		fireEvent.pointerEnter(trigger);
		wait(299);
		expect(API.experimental.getChat).not.toHaveBeenCalled();
		wait(1);
		expect(API.experimental.getChat).toHaveBeenCalledTimes(1);
		expect(API.experimental.getChat).toHaveBeenCalledWith(MockChat.id);

		fireEvent.pointerLeave(trigger);
		wait(200);
		await invalidate();
		expect(API.experimental.getChat).toHaveBeenCalledTimes(1);
	});

	it("does not fetch when the pointer leaves before the delay", () => {
		const { trigger } = renderInfo();

		fireEvent.pointerEnter(trigger);
		wait(100);
		fireEvent.pointerLeave(trigger);
		wait(1000);
		expect(API.experimental.getChat).not.toHaveBeenCalled();
	});

	it("pins on click so leaving keeps the detail live, and the next click closes it", async () => {
		const { trigger, invalidate } = renderInfo();

		fireEvent.pointerEnter(trigger);
		wait(300);
		fireEvent.click(trigger);
		fireEvent.pointerLeave(trigger);
		wait(1000);
		await invalidate();
		expect(API.experimental.getChat).toHaveBeenCalledTimes(2);

		fireEvent.click(trigger);
		await invalidate();
		expect(API.experimental.getChat).toHaveBeenCalledTimes(2);
	});
});
