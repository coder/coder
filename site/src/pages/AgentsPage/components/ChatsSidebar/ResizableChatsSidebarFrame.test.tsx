import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ResizableChatsSidebarFrame } from "./ResizableChatsSidebarFrame";
import {
	LEFT_SIDEBAR_DEFAULT_WIDTH,
	LEFT_SIDEBAR_STORAGE_KEY,
} from "./sidebarWidth";

// Pointer events are dispatched with fireEvent because userEvent cannot emit
// lostpointercapture or pointercancel, set isPrimary, or drive a second
// pointer ID.
const PRIMARY = { pointerId: 1, button: 0, isPrimary: true };
const SECONDARY_POINTER = { pointerId: 2, button: 0, isPrimary: false };
const RIGHT_BUTTON = { pointerId: 1, button: 2, isPrimary: true };

const persistedWidth = () =>
	Number(localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY));

const renderHandle = () => {
	render(
		<ResizableChatsSidebarFrame>
			<div>sidebar</div>
		</ResizableChatsSidebarFrame>,
	);
	return screen.getByTestId("agents-sidebar-resize-handle");
};

describe("ResizableChatsSidebarFrame", () => {
	beforeEach(() => {
		vi.stubGlobal("innerWidth", 1440);
		localStorage.clear();
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it("persists the width while a primary drag is in progress", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...PRIMARY, clientX: 0 });
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 40 });

		expect(persistedWidth()).toBe(LEFT_SIDEBAR_DEFAULT_WIDTH + 40);
	});

	it("stops tracking after lostpointercapture without pointerup", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...PRIMARY, clientX: 0 });
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 40 });
		fireEvent.lostPointerCapture(handle, PRIMARY);
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 200 });

		expect(persistedWidth()).toBe(LEFT_SIDEBAR_DEFAULT_WIDTH + 40);
	});

	it("stops tracking after pointercancel", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...PRIMARY, clientX: 0 });
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 40 });
		fireEvent.pointerCancel(handle, PRIMARY);
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 200 });

		expect(persistedWidth()).toBe(LEFT_SIDEBAR_DEFAULT_WIDTH + 40);
	});

	it("ignores a secondary-button pointerdown", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...RIGHT_BUTTON, clientX: 0 });
		fireEvent.pointerMove(handle, { ...RIGHT_BUTTON, clientX: 40 });

		expect(localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY)).toBeNull();
	});

	it("ignores a non-primary pointer", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...SECONDARY_POINTER, clientX: 0 });
		fireEvent.pointerMove(handle, { ...SECONDARY_POINTER, clientX: 40 });

		expect(localStorage.getItem(LEFT_SIDEBAR_STORAGE_KEY)).toBeNull();
	});

	it("ignores a second pointer while a drag is in progress", () => {
		const handle = renderHandle();

		fireEvent.pointerDown(handle, { ...PRIMARY, clientX: 0 });
		fireEvent.pointerDown(handle, { ...SECONDARY_POINTER, clientX: 0 });
		fireEvent.pointerMove(handle, { ...SECONDARY_POINTER, clientX: 200 });
		fireEvent.pointerUp(handle, SECONDARY_POINTER);
		fireEvent.pointerMove(handle, { ...PRIMARY, clientX: 40 });

		expect(persistedWidth()).toBe(LEFT_SIDEBAR_DEFAULT_WIDTH + 40);
	});
});
