import { renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { isMac } from "#/utils/platform";
import { useAgentsPageKeybindings } from "./useAgentsPageKeybindings";

vi.mock("#/utils/platform", async (importOriginal) => ({
	...(await importOriginal<typeof import("#/utils/platform")>()),
	isMac: vi.fn(),
}));

const isMacMock = vi.mocked(isMac);

type Options = Parameters<typeof useAgentsPageKeybindings>[0];

const renderKeybindings = (
	options: Omit<Options, "vimNavigationEnabled" | "vimModifier"> &
		Partial<Pick<Options, "vimNavigationEnabled" | "vimModifier">>,
) =>
	renderHook(() =>
		useAgentsPageKeybindings({
			vimNavigationEnabled: false,
			vimModifier: "ctrl",
			...options,
		}),
	);

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

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
		});

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

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
		});

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

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
		});

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

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
		});

		const searchEvent = dispatchKeyDown("k", { ctrlKey: true }, input);
		const newAgentEvent = dispatchKeyDown("n", { ctrlKey: true }, input);

		expect(searchEvent.defaultPrevented).toBe(true);
		expect(newAgentEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(1);
		expect(onNewAgent).toHaveBeenCalledTimes(1);

		input.remove();
	});

	it("ignores Ctrl+/, Ctrl+Shift+O, and Ctrl+Shift+E when vim navigation is off", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();
		const onRenameActiveChat = vi.fn();

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
			onRenameActiveChat,
		});

		const slashEvent = dispatchKeyDown("/", { ctrlKey: true });
		const newEvent = dispatchKeyDown("O", { ctrlKey: true, shiftKey: true });
		const renameEvent = dispatchKeyDown("E", { ctrlKey: true, shiftKey: true });

		expect(slashEvent.defaultPrevented).toBe(false);
		expect(newEvent.defaultPrevented).toBe(false);
		expect(renameEvent.defaultPrevented).toBe(false);
		expect(onNewAgent).not.toHaveBeenCalled();
		expect(onToggleSearch).not.toHaveBeenCalled();
		expect(onRenameActiveChat).not.toHaveBeenCalled();
	});

	it("moves search from Ctrl+K to Ctrl+/ when vim navigation is on", () => {
		isMacMock.mockReturnValue(false);
		const onToggleSearch = vi.fn();

		renderKeybindings({
			onNewAgent: vi.fn(),
			onToggleSearch,
			vimNavigationEnabled: true,
		});

		const kEvent = dispatchKeyDown("k", { ctrlKey: true });
		const slashEvent = dispatchKeyDown("/", { ctrlKey: true });
		// Layouts where "/" is a shifted key report shiftKey alongside it.
		const shiftedSlashEvent = dispatchKeyDown("/", {
			ctrlKey: true,
			shiftKey: true,
		});

		expect(kEvent.defaultPrevented).toBe(false);
		expect(slashEvent.defaultPrevented).toBe(true);
		expect(shiftedSlashEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(2);
	});

	it("renames the active chat with Ctrl+Shift+E when vim navigation is on", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onRenameActiveChat = vi.fn();

		renderKeybindings({
			onNewAgent,
			onRenameActiveChat,
			vimNavigationEnabled: true,
		});

		const renameEvent = dispatchKeyDown("E", { ctrlKey: true, shiftKey: true });
		const shiftNEvent = dispatchKeyDown("N", { ctrlKey: true, shiftKey: true });

		expect(renameEvent.defaultPrevented).toBe(true);
		expect(onRenameActiveChat).toHaveBeenCalledTimes(1);
		expect(shiftNEvent.defaultPrevented).toBe(false);
		expect(onNewAgent).not.toHaveBeenCalled();
	});

	it("creates a new agent with Ctrl+Shift+O when vim navigation is on", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();

		renderKeybindings({ onNewAgent, vimNavigationEnabled: true });

		const event = dispatchKeyDown("O", { ctrlKey: true, shiftKey: true });

		expect(event.defaultPrevented).toBe(true);
		expect(onNewAgent).toHaveBeenCalledTimes(1);
	});

	it("binds vim shortcuts to the Alt modifier, including Option chords on macOS", () => {
		isMacMock.mockReturnValue(true);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();
		const onRenameActiveChat = vi.fn();

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
			onRenameActiveChat,
			vimNavigationEnabled: true,
			vimModifier: "alt",
		});

		// Option+/ and Option+Shift+O on a macOS US layout report "÷" and "Ø".
		const slashEvent = dispatchKeyDown("÷", { altKey: true, code: "Slash" });
		const newEvent = dispatchKeyDown("Ø", {
			altKey: true,
			shiftKey: true,
			code: "KeyO",
		});
		const renameEvent = dispatchKeyDown("E", {
			altKey: true,
			shiftKey: true,
			code: "KeyE",
		});
		const metaSlashEvent = dispatchKeyDown("/", { metaKey: true });
		const metaNEvent = dispatchKeyDown("n", { metaKey: true });

		expect(slashEvent.defaultPrevented).toBe(true);
		expect(newEvent.defaultPrevented).toBe(true);
		expect(renameEvent.defaultPrevented).toBe(true);
		expect(metaSlashEvent.defaultPrevented).toBe(false);
		expect(metaNEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(1);
		expect(onRenameActiveChat).toHaveBeenCalledTimes(1);
		expect(onNewAgent).toHaveBeenCalledTimes(2);
	});

	it("keeps Ctrl+K search when the vim modifier is not the platform modifier", () => {
		isMacMock.mockReturnValue(false);
		const onNewAgent = vi.fn();
		const onToggleSearch = vi.fn();

		renderKeybindings({
			onNewAgent,
			onToggleSearch,
			vimNavigationEnabled: true,
			vimModifier: "meta",
		});

		const metaSlashEvent = dispatchKeyDown("/", { metaKey: true });
		const ctrlSlashEvent = dispatchKeyDown("/", { ctrlKey: true });
		const metaShiftOEvent = dispatchKeyDown("O", {
			metaKey: true,
			shiftKey: true,
		});
		const ctrlKEvent = dispatchKeyDown("k", { ctrlKey: true });

		expect(metaSlashEvent.defaultPrevented).toBe(true);
		expect(ctrlSlashEvent.defaultPrevented).toBe(false);
		expect(metaShiftOEvent.defaultPrevented).toBe(true);
		expect(ctrlKEvent.defaultPrevented).toBe(true);
		expect(onToggleSearch).toHaveBeenCalledTimes(2);
		expect(onNewAgent).toHaveBeenCalledTimes(1);
	});
});
