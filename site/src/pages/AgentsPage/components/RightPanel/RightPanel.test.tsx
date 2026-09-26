import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FC, useState } from "react";
import { MemoryRouter, Outlet, Route, Routes } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentsPageOutletContext } from "../../AgentsPageLayout";
import {
	RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH,
	RIGHT_PANEL_WIDTH_KEY,
	RightPanel,
} from "./RightPanel";

type HarnessProps = {
	onOpenChange?: (isOpen: boolean) => void;
	onExpandedChange?: (isExpanded: boolean) => void;
	onVisualExpandedChange?: (visualExpanded: boolean | null) => void;
};

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
		<>
			<button type="button" onClick={() => setIsOpen(true)}>
				Open panel
			</button>
			<RightPanel
				isOpen={isOpen}
				isExpanded={isOpen && isExpanded}
				onToggleExpanded={() => setIsExpanded(!isExpanded)}
				onClose={() => setIsOpen(false)}
				onVisualExpandedChange={onVisualExpandedChange}
			>
				<div>Panel content</div>
			</RightPanel>
		</>
	);
};

type SidebarHarnessProps = HarnessProps & {
	onSidebarCollapsedChange?: (isCollapsed: boolean) => void;
};

/**
 * Supplies the sidebar outlet context the panel reads and writes, plus
 * "Expand sidebar" and "Collapse sidebar" buttons that change the sidebar
 * the way a user click does.
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
			<Route
				element={
					<>
						<button type="button" onClick={() => setIsSidebarCollapsed(false)}>
							Expand sidebar
						</button>
						<button type="button" onClick={() => setIsSidebarCollapsed(true)}>
							Collapse sidebar
						</button>
						<Outlet context={outletContext} />
					</>
				}
			>
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

	describe("side-by-side room check", () => {
		// Room checks only run at or above the side-by-side breakpoint.
		const SIDE_BY_SIDE_VIEWPORT =
			RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH + 176;
		const NARROW_VIEWPORT = RIGHT_PANEL_SIDE_BY_SIDE_BREAKPOINT_WIDTH - 124;
		// Enough for the 360px chat minimum plus the 360px panel minimum.
		const ROOMY_PARENT_WIDTH = 2000;

		// jsdom reports a zero-width parent unless clientWidth is stubbed,
		// so by default there is never room for both the chat and the
		// panel. Animation frames run synchronously to keep checks
		// deterministic.
		beforeEach(() => {
			vi.stubGlobal("innerWidth", SIDE_BY_SIDE_VIEWPORT);
			vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
				cb(0);
				return 0;
			});
			vi.stubGlobal("cancelAnimationFrame", () => {});
		});

		afterEach(() => {
			vi.restoreAllMocks();
		});

		const stubParentWidth = (width: number) =>
			vi
				.spyOn(HTMLElement.prototype, "clientWidth", "get")
				.mockReturnValue(width);

		const resizeWindow = (width: number) => {
			vi.stubGlobal("innerWidth", width);
			fireEvent(window, new Event("resize"));
		};

		const renderWithSidebar = () => {
			const onOpenChange = vi.fn();
			const onExpandedChange = vi.fn();
			const onSidebarCollapsedChange = vi.fn();
			render(
				<MemoryRouter>
					<RightPanelWithSidebarHarness
						onOpenChange={onOpenChange}
						onExpandedChange={onExpandedChange}
						onSidebarCollapsedChange={onSidebarCollapsedChange}
					/>
				</MemoryRouter>,
			);
			return { onOpenChange, onExpandedChange, onSidebarCollapsedChange };
		};

		it("collapses the sidebar to make room for the open panel", () => {
			const { onOpenChange, onSidebarCollapsedChange } = renderWithSidebar();

			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(true);
			expect(onOpenChange).not.toHaveBeenCalled();
		});

		it("closes the panel instead of re-collapsing a sidebar the user expanded", async () => {
			const user = userEvent.setup();
			const { onOpenChange, onSidebarCollapsedChange } = renderWithSidebar();
			onSidebarCollapsedChange.mockClear();

			await user.click(screen.getByRole("button", { name: "Expand sidebar" }));

			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(false);
			expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(false);
		});

		it("does not carry a below-breakpoint expand into a later resize", async () => {
			const user = userEvent.setup();
			const { onOpenChange, onSidebarCollapsedChange } = renderWithSidebar();
			resizeWindow(NARROW_VIEWPORT);
			await user.click(screen.getByRole("button", { name: "Expand sidebar" }));
			onSidebarCollapsedChange.mockClear();

			resizeWindow(SIDE_BY_SIDE_VIEWPORT);

			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(true);
			expect(onOpenChange).not.toHaveBeenCalled();
		});

		it("collapses the sidebar when the panel opens after the sidebar was expanded", async () => {
			const user = userEvent.setup();
			const { onOpenChange, onSidebarCollapsedChange } = renderWithSidebar();
			pointerDown();
			pointerMove(CLOSE_ZONE_X);
			pointerUp(CLOSE_ZONE_X);
			await user.click(screen.getByRole("button", { name: "Expand sidebar" }));
			onOpenChange.mockClear();
			onSidebarCollapsedChange.mockClear();

			await user.click(screen.getByRole("button", { name: "Open panel" }));

			expect(onOpenChange).toHaveBeenCalledExactlyOnceWith(true);
			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(true);
		});

		it("collapses the sidebar on a later narrowing after an expand that fit", async () => {
			const user = userEvent.setup();
			const parentWidth = stubParentWidth(ROOMY_PARENT_WIDTH);
			const { onOpenChange, onSidebarCollapsedChange } = renderWithSidebar();
			await user.click(
				screen.getByRole("button", { name: "Collapse sidebar" }),
			);
			await user.click(screen.getByRole("button", { name: "Expand sidebar" }));
			onSidebarCollapsedChange.mockClear();

			parentWidth.mockReturnValue(0);
			resizeWindow(SIDE_BY_SIDE_VIEWPORT);

			expect(onSidebarCollapsedChange).toHaveBeenCalledExactlyOnceWith(true);
			expect(onOpenChange).not.toHaveBeenCalled();
		});

		it("treats a resize drag's sidebar re-expand as the drag's, not the user's", async () => {
			const user = userEvent.setup();
			const { onOpenChange, onExpandedChange, onSidebarCollapsedChange } =
				renderWithSidebar();
			// Expand the panel, then expand the sidebar behind it.
			pointerDown();
			pointerMove(EXPAND_ZONE_X);
			pointerUp(EXPAND_ZONE_X);
			expect(onExpandedChange).toHaveBeenLastCalledWith(true);
			await user.click(screen.getByRole("button", { name: "Expand sidebar" }));
			onSidebarCollapsedChange.mockClear();

			// Touch the left edge, come back, and release in the normal zone.
			pointerDown({ clientX: 1000 });
			pointerMove(SIDEBAR_EDGE_X);
			pointerMove(700);
			pointerUp(700);

			expect(onExpandedChange).toHaveBeenLastCalledWith(false);
			expect(onSidebarCollapsedChange).toHaveBeenLastCalledWith(true);
			expect(onOpenChange).not.toHaveBeenCalled();
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
