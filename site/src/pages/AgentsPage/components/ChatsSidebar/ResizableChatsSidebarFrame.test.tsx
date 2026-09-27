import { fireEvent, render, screen } from "@testing-library/react";
import { createRef } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ResizableChatsSidebarFrame } from "./ResizableChatsSidebarFrame";
import {
	LEFT_SIDEBAR_DEFAULT_WIDTH,
	LEFT_SIDEBAR_STORAGE_KEY,
	readLeftSidebarWidth,
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
		vi.restoreAllMocks();
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
		expect(handle).toHaveAttribute("aria-valuenow", "340");

		vi.stubGlobal("innerWidth", 1440);
		fireEvent(window, new Event("resize"));
		expect(handle).toHaveAttribute("aria-valuenow", "400");
		expect(persistedWidth()).toBe(400);
	});

	it("grows a sidebar squeezed at mount back to the stored width", () => {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "600");
		vi.stubGlobal("innerWidth", 800);
		const handle = renderHandle();
		expect(handle).toHaveAttribute("aria-valuenow", "440");

		vi.stubGlobal("innerWidth", 1440);
		fireEvent(window, new Event("resize"));
		expect(handle).toHaveAttribute("aria-valuenow", "600");
	});

	it("keeps the chosen width across resizes when storage writes fail", () => {
		vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
			throw new Error("quota exceeded");
		});
		const handle = renderHandle();

		fireEvent.keyDown(handle, { key: "End" });
		const chosenWidth = handle.getAttribute("aria-valuenow");
		fireEvent(window, new Event("resize"));

		expect(handle).toHaveAttribute("aria-valuenow", chosenWidth);
	});

	it("reports the end of its own slide but not of a child animation", () => {
		const onViewportSlideEnd = vi.fn();
		render(
			<ResizableChatsSidebarFrame
				viewportSlide="out"
				onViewportSlideEnd={onViewportSlideEnd}
			>
				<div data-testid="child">sidebar</div>
			</ResizableChatsSidebarFrame>,
		);

		fireEvent.animationEnd(screen.getByTestId("child"));
		expect(onViewportSlideEnd).not.toHaveBeenCalled();

		fireEvent.animationEnd(screen.getByTestId("agents-sidebar-panel"));
		expect(onViewportSlideEnd).toHaveBeenCalledOnce();
	});

	it("exposes its expanded width to readLeftSidebarWidth while collapsed", () => {
		localStorage.setItem(LEFT_SIDEBAR_STORAGE_KEY, "400");
		const ref = createRef<HTMLDivElement>();
		render(
			<ResizableChatsSidebarFrame ref={ref} isCollapsed>
				<div>sidebar</div>
			</ResizableChatsSidebarFrame>,
		);

		expect(readLeftSidebarWidth(ref.current)).toBe(400);
	});
});
