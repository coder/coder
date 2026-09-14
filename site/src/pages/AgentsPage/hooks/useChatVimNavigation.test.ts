import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useChatVimNavigation } from "./useChatVimNavigation";

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
			modifier: "ctrl",
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
		document.body.innerHTML = "";
	});

	it("does nothing when disabled", () => {
		const onSelectChat = render({ enabled: false });

		const event = dispatchKeyDown("j", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(false);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("moves to the next and previous chat with Ctrl+J and Ctrl+K", () => {
		const onSelectChat = render();

		const nextEvent = dispatchKeyDown("j", { ctrlKey: true });
		const prevEvent = dispatchKeyDown("k", { ctrlKey: true });

		expect(nextEvent.defaultPrevented).toBe(true);
		expect(prevEvent.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "a");
	});

	it("matches only the configured modifier", () => {
		const onSelectChat = render({ modifier: "meta" });

		const ctrlEvent = dispatchKeyDown("j", { ctrlKey: true });
		const altEvent = dispatchKeyDown("j", { altKey: true });
		const metaEvent = dispatchKeyDown("j", { metaKey: true });

		expect(ctrlEvent.defaultPrevented).toBe(false);
		expect(altEvent.defaultPrevented).toBe(false);
		expect(metaEvent.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("c");
	});

	it("navigates with the Alt modifier, including Option+J on macOS", () => {
		const onSelectChat = render({ modifier: "alt" });

		// Option+J on a macOS US layout reports the character "∆".
		const nextEvent = dispatchKeyDown("∆", { altKey: true, code: "KeyJ" });
		const prevEvent = dispatchKeyDown("k", { altKey: true, code: "KeyK" });

		expect(nextEvent.defaultPrevented).toBe(true);
		expect(prevEvent.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "a");
	});

	it("ignores chords with an extra modifier held", () => {
		const onSelectChat = render();

		const event = dispatchKeyDown("j", { ctrlKey: true, altKey: true });

		expect(event.defaultPrevented).toBe(false);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("jumps to the last and first chat with Shift", () => {
		const onSelectChat = render({ activeChatId: "b" });

		dispatchKeyDown("J", { ctrlKey: true, shiftKey: true });
		dispatchKeyDown("K", { ctrlKey: true, shiftKey: true });

		expect(onSelectChat).toHaveBeenNthCalledWith(1, "c");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "a");
	});

	it("anchors a hidden active chat to its nearest visible neighbors", () => {
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
		const onSelectChat = render({
			visibleChatIds: ["a", "b"],
			allChatIds: ["a", "b", "c"],
			activeChatId: "c",
		});

		dispatchKeyDown("j", { ctrlKey: true });

		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("b");
	});

	it("ignores keys while focus is inside a dialog", () => {
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

	it("matches the physical key on non-Latin layouts", () => {
		const onSelectChat = render();

		// Russian layout: the key at the "J" position reports "о".
		const event = dispatchKeyDown("о", { ctrlKey: true, code: "KeyJ" });

		expect(event.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("c");
	});

	it("stops at the list boundaries", () => {
		const onSelectChat = render({ activeChatId: "c" });

		const event = dispatchKeyDown("j", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onSelectChat).not.toHaveBeenCalled();
	});

	it("enters the list from either end when no chat is active", () => {
		const onSelectChat = render({ activeChatId: undefined });

		dispatchKeyDown("j", { ctrlKey: true });
		dispatchKeyDown("k", { ctrlKey: true });

		expect(onSelectChat).toHaveBeenNthCalledWith(1, "a");
		expect(onSelectChat).toHaveBeenNthCalledWith(2, "c");
	});

	it("handles shortcuts from editable elements", () => {
		const onSelectChat = render();
		const input = document.createElement("input");
		document.body.appendChild(input);

		const event = dispatchKeyDown("j", { ctrlKey: true }, input);

		expect(event.defaultPrevented).toBe(true);
		expect(onSelectChat).toHaveBeenCalledExactlyOnceWith("c");
	});

	it("focuses the composer on Escape from a sidebar row", () => {
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
		render();
		const composer = document.createElement("div");
		composer.dataset.testid = "chat-message-input";
		document.body.append(composer);

		const event = dispatchKeyDown("Escape");

		expect(event.defaultPrevented).toBe(false);
	});
});
