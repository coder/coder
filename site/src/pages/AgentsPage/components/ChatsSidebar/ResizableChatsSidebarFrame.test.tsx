import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ResizableChatsSidebarFrame } from "./ResizableChatsSidebarFrame";
import {
	AGENTS_MAIN_PANEL_MIN_WIDTH,
	getLeftSidebarMaxWidth,
	LEFT_SIDEBAR_DEFAULT_WIDTH,
	LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
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

	it("restores the persisted width after a narrow window widens", () => {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "400");
		const handle = renderHandle();

		vi.stubGlobal("innerWidth", 700);
		fireEvent(window, new Event("resize"));
		expect(handle).toHaveAttribute(
			"aria-valuenow",
			String(700 - AGENTS_MAIN_PANEL_MIN_WIDTH),
		);

		vi.stubGlobal("innerWidth", 1440);
		fireEvent(window, new Event("resize"));
		expect(handle).toHaveAttribute("aria-valuenow", "400");
		expect(persistedWidth()).toBe(400);
	});

	it("lifts the max width when a sidebar squeezed at mount grows back", () => {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "600");
		vi.stubGlobal("innerWidth", 800);
		const handle = renderHandle();
		expect(handle).toHaveAttribute(
			"aria-valuemax",
			String(800 - AGENTS_MAIN_PANEL_MIN_WIDTH),
		);

		vi.stubGlobal("innerWidth", 1440);
		fireEvent(window, new Event("resize"));
		expect(handle).toHaveAttribute("aria-valuenow", "600");
		expect(Number(handle.getAttribute("aria-valuemax"))).toBeGreaterThanOrEqual(
			600,
		);
	});

	it("keeps a resized width across window resizes", () => {
		const handle = renderHandle();

		fireEvent.keyDown(handle, { key: "End" });
		const resizedWidth = handle.getAttribute("aria-valuenow");
		expect(resizedWidth).toBe(String(getLeftSidebarMaxWidth()));
		fireEvent(window, new Event("resize"));

		expect(handle).toHaveAttribute("aria-valuenow", resizedWidth);
	});

	it.each([
		{ key: "ArrowRight", expected: 500 },
		{ key: "End", expected: 500 },
		{
			key: "ArrowLeft",
			expected:
				700 - AGENTS_MAIN_PANEL_MIN_WIDTH - LEFT_SIDEBAR_KEYBOARD_RESIZE_STEP,
		},
	])(
		"saves $expected when $key resizes a squeezed sidebar",
		({ key, expected }) => {
			localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "500");
			const handle = renderHandle();

			vi.stubGlobal("innerWidth", 700);
			fireEvent(window, new Event("resize"));
			fireEvent.keyDown(handle, { key });
			vi.stubGlobal("innerWidth", 1440);
			fireEvent(window, new Event("resize"));

			expect(handle).toHaveAttribute("aria-valuenow", String(expected));
			expect(persistedWidth()).toBe(expected);
		},
	);

	it("reports the end of its current slide only", () => {
		const onViewportSlideEnd = vi.fn();
		render(
			<ResizableChatsSidebarFrame
				viewportSlide="out"
				onViewportSlideEnd={onViewportSlideEnd}
			>
				<div data-testid="child">sidebar</div>
			</ResizableChatsSidebarFrame>,
		);

		// jsdom has no AnimationEvent, so set animationName on a plain event.
		const animationEnd = (target: Element, animationName: string) =>
			fireEvent(
				target,
				Object.assign(new Event("animationend", { bubbles: true }), {
					animationName,
				}),
			);
		const panel = screen.getByTestId("agents-sidebar-panel");
		animationEnd(screen.getByTestId("child"), "panel-slide-out");
		animationEnd(panel, "panel-slide-in");
		expect(onViewportSlideEnd).not.toHaveBeenCalled();

		animationEnd(panel, "panel-slide-out");
		expect(onViewportSlideEnd).toHaveBeenCalledOnce();
	});
});
