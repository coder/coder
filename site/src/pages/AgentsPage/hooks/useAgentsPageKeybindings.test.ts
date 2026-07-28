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

	it("toggles search with Ctrl+K on non-macOS", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onToggleSearch,
			}),
		);

		const firstEvent = dispatchKeyDown("k", { ctrlKey: true });
		const secondEvent = dispatchKeyDown("k", { ctrlKey: true });

		expect(firstEvent.defaultPrevented).toBe(true);
		expect(secondEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(2);
		expect(onNewAgent).not.toHaveBeenCalled();
	});

	it("uses Cmd instead of Ctrl on macOS", () => {
		isMacMock.mockReturnValue(true);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onToggleSearch,
			}),
		);

		const ctrlEvent = dispatchKeyDown("k", { ctrlKey: true });
		const metaEvent = dispatchKeyDown("k", { metaKey: true });

		expect(ctrlEvent.defaultPrevented).toBe(false);
		expect(metaEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(1);
	});

	it("creates a new agent with Ctrl+N", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onToggleSearch,
			}),
		);

		const event = dispatchKeyDown("n", { ctrlKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onNewAgent).toHaveBeenCalledTimes(1);
		expect(onToggleSearch).not.toHaveBeenCalled();
	});

	it("handles shortcuts from editable elements", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();
		const input = document.createElement("input");
		document.body.appendChild(input);

		renderHook(() =>
			useAgentsPageKeybindings({
				onNewAgent,
				onToggleSearch,
			}),
		);

		const searchEvent = dispatchKeyDown("k", { ctrlKey: true }, input);
		const newAgentEvent = dispatchKeyDown("n", { ctrlKey: true }, input);

		expect(searchEvent.defaultPrevented).toBe(true);
		expect(newAgentEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(1);
		expect(onNewAgent).toHaveBeenCalledTimes(1);

		input.remove();
	});
});
