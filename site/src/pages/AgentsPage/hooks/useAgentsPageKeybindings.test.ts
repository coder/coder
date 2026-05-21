import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isMac } from "#/utils/platform";
import { useAgentsPageKeybindings } from "./useAgentsPageKeybindings";

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

describe("useAgentsPageKeybindings", () => {
	afterEach(() => {
		vi.clearAllMocks();
	});

	it("opens search with Ctrl+K on non-macOS", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onOpenSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onOpenSearch,
			}),
		);

		const event = dispatchKeyDown("k", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onOpenSearch).toHaveBeenCalledTimes(1);
		expect(onNewAgent).not.toHaveBeenCalled();
	});

	it("uses Cmd instead of Ctrl on macOS", () => {
		isMacMock.mockReturnValue(true);
		const onNewAgent = vi.fn();
		const onOpenSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onOpenSearch,
			}),
		);

		const ctrlEvent = dispatchKeyDown("k", { ctrlKey: true });
		const metaEvent = dispatchKeyDown("k", { metaKey: true });

		expect(ctrlEvent.defaultPrevented).toBe(false);
		expect(metaEvent.defaultPrevented).toBe(true);
		expect(onOpenSearch).toHaveBeenCalledTimes(1);
	});

	it("creates a new agent with Ctrl+N", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onOpenSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onOpenSearch,
			}),
		);

		const event = dispatchKeyDown("n", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onNewAgent).toHaveBeenCalledTimes(1);
		expect(onOpenSearch).not.toHaveBeenCalled();
	});

	it("ignores shortcuts from editable elements", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onOpenSearch = vi.fn();
		const input = document.createElement("input");
		document.body.appendChild(input);

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onOpenSearch,
			}),
		);

		const event = dispatchKeyDown("k", { ctrlKey: true }, input);

		expect(event.defaultPrevented).toBe(false);
		expect(onOpenSearch).not.toHaveBeenCalled();
		expect(onNewAgent).not.toHaveBeenCalled();

		input.remove();
	});
});
