import { fireEvent, render, screen } from "@testing-library/react";
import { type FC, useState } from "react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import { RIGHT_PANEL_WIDTH_KEY, RightPanel } from "./RightPanel";

interface HarnessProps {
	onOpenChange?: (isOpen: boolean) => void;
	onExpandedChange?: (isExpanded: boolean) => void;
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
}

/**
 * Owns the open and expanded state around a RightPanel the way the chat
 * page does and reports every transition so tests can assert on them.
 */
const RightPanelHarness: FC<HarnessProps> = ({
	onOpenChange,
	onExpandedChange,
	onVisualExpandedChange,
}) => {
	const [isOpen, setIsOpenState] = useState(true);
	const [isExpanded, setIsExpandedState] = useState(false);
	const setIsOpen = (next: boolean) => {
		setIsOpenState(next);
		onOpenChange?.(next);
	};
	const setIsExpanded = (next: boolean) => {
		setIsExpandedState(next);
		onExpandedChange?.(next);
	};

	return (
		<RightPanel
			isOpen={isOpen}
			isExpanded={isOpen && isExpanded}
			onToggleExpanded={() => setIsExpanded(!isExpanded)}
			onClose={() => setIsOpen(false)}
			onVisualExpandedChange={onVisualExpandedChange}
		>
			<div>Panel content</div>
		</RightPanel>
	);
};

interface SidebarHarnessProps extends HarnessProps {
	onSidebarCollapsedChange?: (isCollapsed: boolean) => void;
}

/**
 * Supplies the outlet context the panel uses to collapse the chats
 * sidebar while the pointer is at the left edge of the viewport.
 */
const RightPanelWithSidebarHarness: FC<SidebarHarnessProps> = ({
	onSidebarCollapsedChange,
	...harnessProps
}) => {
	const [isSidebarCollapsed, setIsSidebarCollapsedState] = useState(false);
	const setIsSidebarCollapsed = (next: boolean) => {
		setIsSidebarCollapsedState(next);
		onSidebarCollapsedChange?.(next);
	};
	const outletContext: AgentsPageOutletContext = {
		chatErrorReasons: {},
		setChatErrorReason: () => {},
		clearChatErrorReason: () => {},
		requestArchiveAgent: () => {},
		requestUnarchiveAgent: () => {},
		requestArchiveAndDeleteWorkspace: () => {},
		requestPinAgent: () => {},
		requestUnpinAgent: () => {},
		isArchiving: false,
		archivingChatId: undefined,
		activeChatChildren: undefined,
		isSidebarCollapsed,
		onToggleSidebarCollapsed: () => setIsSidebarCollapsed(!isSidebarCollapsed),
		onExpandSidebar: () => setIsSidebarCollapsed(false),
		onChatReady: () => {},
	};

	return (
		<Routes>
			<Route element={<Outlet context={outletContext} />}>
				<Route path="*" element={<RightPanelHarness {...harnessProps} />} />
			</Route>
		</Routes>
	);
};

// jsdom lays nothing out: the panel's rect is 0 wide and its parent has
// no client width. The viewport is pinned below the side-by-side
// breakpoint so the max width comes from innerWidth alone (700px) and
// the initial 480px width is not clamped on mount. With a zero start
// width the raw drag width is -clientX, giving these zones:
const CLOSE_ZONE_X = 100; // raw -100 < 280
const NORMAL_ZONE_X = -600; // raw 600, kept as the live width
const EXPAND_ZONE_X = -900; // raw 900 > 700 + 80
const SIDEBAR_EDGE_X = 10; // below the 80px left-edge threshold

// Pointer events are dispatched with fireEvent: user-event cannot emit
// lostpointercapture or pointercancel, set isPrimary, or drive a second
// pointer id, and the capture methods are stubbed globally for jsdom.
const pointer = { pointerId: 1, button: 0, isPrimary: true };

const persistedWidth = () => localStorage.getItem(RIGHT_PANEL_WIDTH_KEY);

const getHandle = () => screen.getByTestId("agents-right-panel-resize-handle");

const pointerDown = (init: Partial<PointerEventInit> = {}) =>
	fireEvent.pointerDown(getHandle(), { ...pointer, clientX: 0, ...init });
const pointerMove = (clientX: number, init: Partial<PointerEventInit> = {}) =>
	fireEvent.pointerMove(getHandle(), { ...pointer, clientX, ...init });
const pointerUp = (clientX: number, init: Partial<PointerEventInit> = {}) =>
	fireEvent.pointerUp(getHandle(), { ...pointer, clientX, ...init });
const pointerCancel = (clientX: number) =>
	fireEvent.pointerCancel(getHandle(), { ...pointer, clientX });
const lostPointerCapture = (clientX: number) =>
	fireEvent.lostPointerCapture(getHandle(), { ...pointer, clientX });

const renderHarness = () => {
	const onOpenChange = vi.fn();
	const onExpandedChange = vi.fn();
	const onVisualExpandedChange = vi.fn();
	render(
		<MemoryRouter>
			<RightPanelHarness
				onOpenChange={onOpenChange}
				onExpandedChange={onExpandedChange}
				onVisualExpandedChange={onVisualExpandedChange}
			/>
		</MemoryRouter>,
	);
	return { onOpenChange, onExpandedChange, onVisualExpandedChange };
};

beforeEach(() => {
	localStorage.removeItem(RIGHT_PANEL_WIDTH_KEY);
	vi.stubGlobal("innerWidth", 1000);
});

afterEach(() => {
	vi.unstubAllGlobals();
	localStorage.removeItem(RIGHT_PANEL_WIDTH_KEY);
});

describe("RightPanel resize drag", () => {
	describe("a drag that ends without pointerup", () => {
		// The abort must clear the visual override and leave the open and
		// expanded state to their owners, without committing the snap.
		it("clears the live-expanded override and does not close the panel", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(NORMAL_ZONE_X);
			pointerMove(CLOSE_ZONE_X);
			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(false);

			lostPointerCapture(CLOSE_ZONE_X);

			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(null);
			expect(onOpenChange).not.toHaveBeenCalled();
		});

		it("keeps the live width instead of resetting it", () => {
			renderHarness();

			pointerDown();
			pointerMove(NORMAL_ZONE_X);
			pointerMove(CLOSE_ZONE_X);
			lostPointerCapture(CLOSE_ZONE_X);

			expect(persistedWidth()).toBe("600");
		});

		it("handles pointercancel the same way", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(NORMAL_ZONE_X);
			pointerCancel(NORMAL_ZONE_X);

			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(null);
			expect(onOpenChange).not.toHaveBeenCalled();
			expect(persistedWidth()).toBe("600");
		});

		it("does not commit an expanded snap", () => {
			const { onExpandedChange, onVisualExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(EXPAND_ZONE_X);
			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(true);

			lostPointerCapture(EXPAND_ZONE_X);

			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(null);
			expect(onExpandedChange).not.toHaveBeenCalled();
		});

		it("re-expands a sidebar the same drag collapsed", () => {
			const onSidebarCollapsedChange = vi.fn();
			render(
				<MemoryRouter>
					<RightPanelWithSidebarHarness
						onSidebarCollapsedChange={onSidebarCollapsedChange}
					/>
				</MemoryRouter>,
			);

			pointerDown();
			pointerMove(SIDEBAR_EDGE_X);
			expect(onSidebarCollapsedChange).toHaveBeenLastCalledWith(true);

			lostPointerCapture(SIDEBAR_EDGE_X);

			expect(onSidebarCollapsedChange).toHaveBeenLastCalledWith(false);
		});
	});

	describe("a drag that ends with pointerup", () => {
		it("commits the close and resets the width", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(NORMAL_ZONE_X);
			pointerMove(CLOSE_ZONE_X);
			pointerUp(CLOSE_ZONE_X);

			expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(null);
			expect(persistedWidth()).toBe("480");
		});

		it("ignores the lostpointercapture that follows the release", () => {
			const { onOpenChange } = renderHarness();

			pointerDown();
			pointerMove(CLOSE_ZONE_X);
			pointerUp(CLOSE_ZONE_X);
			lostPointerCapture(CLOSE_ZONE_X);

			expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
		});

		it("commits the expanded snap", () => {
			const { onExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(EXPAND_ZONE_X);
			pointerUp(EXPAND_ZONE_X);

			expect(onExpandedChange).toHaveBeenCalledExactlyOnceWith(true);
		});

		it("keeps a sidebar collapse the drag caused", () => {
			const onSidebarCollapsedChange = vi.fn();
			render(
				<MemoryRouter>
					<RightPanelWithSidebarHarness
						onSidebarCollapsedChange={onSidebarCollapsedChange}
					/>
				</MemoryRouter>,
			);

			pointerDown();
			pointerMove(SIDEBAR_EDGE_X);
			pointerUp(SIDEBAR_EDGE_X);

			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(true);
		});
	});

	describe("pointerdown guards", () => {
		// A press that must not start a drag leaves the release below with
		// nothing to commit.
		it("ignores a secondary-button press", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown({ button: 2 });
			pointerMove(CLOSE_ZONE_X);
			pointerUp(CLOSE_ZONE_X, { button: 2 });

			expect(onOpenChange).not.toHaveBeenCalled();
			expect(onVisualExpandedChange).not.toHaveBeenCalled();
			expect(persistedWidth()).toBe("480");
		});

		it("ignores a non-primary pointer", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown({ isPrimary: false });
			pointerMove(CLOSE_ZONE_X);
			pointerUp(CLOSE_ZONE_X);

			expect(onOpenChange).not.toHaveBeenCalled();
			expect(onVisualExpandedChange).not.toHaveBeenCalled();
		});

		it("ignores a second pointer while a drag is active", () => {
			const { onOpenChange, onVisualExpandedChange } = renderHarness();

			pointerDown();
			pointerMove(NORMAL_ZONE_X);
			pointerDown({ pointerId: 2, clientX: 500 });
			pointerMove(CLOSE_ZONE_X, { pointerId: 2 });
			pointerUp(CLOSE_ZONE_X, { pointerId: 2 });

			expect(onOpenChange).not.toHaveBeenCalled();
			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(false);

			pointerUp(NORMAL_ZONE_X);

			expect(onVisualExpandedChange).toHaveBeenLastCalledWith(null);
			expect(persistedWidth()).toBe("600");
		});
	});
});
