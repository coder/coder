import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isMac } from "#/utils/platform";
import { useChatVimNavigation } from "./useChatVimNavigation";

vi.mock("#/utils/platform", () => ({
	isMac: vi.fn(),
}));

const isMacMock = vi.mocked(isMac);

const dispatchKeyDown = (
	key: string,
	options: KeyboardEventInit = {},
	target: EventTarget = document,
) => {
	const event = new KeyboardEvent("keydown", {
		key,
		cancelable: true,
		bubbles: true,
		...options,
	});
	target.dispatchEvent(event);
	return event;
};

const chatIds = ["a", "b", "c"];

const render = (
	overrides: Partial<Parameters<typeof useChatVimNavigation>[0]> = {},
) => {
	const onSelectChat = vi.fn();
	renderHook(() =>
		useChatVimNavigation({
			enabled: true,
			visibleChatIds: chatIds,
			allChatIds: chatIds,
			activeChatId: "b",
			onSelectChat,
			...overrides,
		}),
	);
	return onSelectChat;
};

describe("useChatVimNavigation", () => {
	afterEach(() => {
		vi.clearAllMocks();
		document.body.innerHTML = "";
	});

	it("does nothing when disabled", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render({ enabled: false });

		const event = dispatchKeyDown("j", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(false);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("moves to the next and previous chat with Ctrl+J and Ctrl+K", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render();

		const nextEvent = dispatchKeyDown("j", { ctrlKey: true });
		const prevEvent = dispatchKeyDown("k", { ctrlKey: true });

		expect(nextEvent.defaultPrevented).toBe(true);
		expect(prevEvent.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "a");
	});

	it("uses Cmd instead of Ctrl on macOS", () => {
		isMacMock.mockReturnValue(true);
		const onSelectChat = render();

		const ctrlEvent = dispatchKeyDown("j", { ctrlKey: true });
		const metaEvent = dispatchKeyDown("j", { metaKey: true });

		expect(ctrlEvent.defaultPrevented).toBe(false);
		expect(metaEvent.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("c");
	});

	it("jumps to the last and first chat with Shift", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render({ activeChatId: "b" });

		dispatchKeyDown("J", { ctrlKey: true, shiftKey: true });
		dispatchKeyDown("K", { ctrlKey: true, shiftKey: true });

		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "a");
	});

	it("anchors a hidden active chat to its nearest visible neighbors", () => {
		isMacMock.mockReturnValue(false);
		// "b1" is a collapsed child of "b", so it is in the full order
		// but not the visible one.
		const onSelectChat = render({
			visibleChatIds: ["a", "b", "c"],
			allChatIds: ["a", "b", "b1", "c"],
			activeChatId: "b1",
		});

		dispatchKeyDown("j", { ctrlKey: true });
		dispatchKeyDown("k", { ctrlKey: true });

		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "b");
	});

	it("clamps a hidden active chat at the list edges", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render({
			visibleChatIds: ["a", "b"],
			allChatIds: ["a", "b", "c"],
			activeChatId: "c",
		});

		dispatchKeyDown("j", { ctrlKey: true });

		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("b");
	});

	it("ignores keys while focus is inside a dialog", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render();
		const dialog = document.createElement("div");
		dialog.setAttribute("role", "dialog");
		const input = document.createElement("input");
		dialog.appendChild(input);
		document.body.appendChild(dialog);
		input.focus();

		const event = dispatchKeyDown("j", { ctrlKey: true }, input);

		expect(event.defaultPrevented).toBe(false);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("stops at the list boundaries", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render({ activeChatId: "c" });

		const event = dispatchKeyDown("j", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("enters the list from either end when no chat is active", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render({ activeChatId: undefined });

		dispatchKeyDown("j", { ctrlKey: true });
		dispatchKeyDown("k", { ctrlKey: true });

		expect(onSelectChat).toHaveBeenNthCalledWith(1, "a");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "c");
	});

	it("handles shortcuts from editable elements", () => {
		isMacMock.mockReturnValue(false);
		const onSelectChat = render();
		const input = document.createElement("input");
		document.body.appendChild(input);

		const event = dispatchKeyDown("j", { ctrlKey: true }, input);

		expect(event.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("c");
	});

	it("focuses the composer on Escape from a sidebar row", () => {
		isMacMock.mockReturnValue(false);
		render();
		const row = document.createElement("div");
		row.dataset.testid = "agents-tree-node-b";
		const link = document.createElement("a");
		link.href = "#";
		row.appendChild(link);
		const composer = document.createElement("div");
		composer.dataset.testid = "chat-message-input";
		composer.tabIndex = 0;
		document.body.append(row, composer);
		link.focus();

		const event = dispatchKeyDown("Escape", {}, link);

		expect(event.defaultPrevented).toBe(true);
		expect(document.activeElement).toBe(composer);
	});

	it("ignores Escape outside the sidebar", () => {
		isMacMock.mockReturnValue(false);
		render();
		const composer = document.createElement("div");
		composer.dataset.testid = "chat-message-input";
		document.body.append(composer);

		const event = dispatchKeyDown("Escape");

		expect(event.defaultPrevented).toBe(false);
	});
});
