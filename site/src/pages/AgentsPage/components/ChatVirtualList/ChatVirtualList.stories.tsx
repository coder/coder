import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useRef, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { ChatVirtualList, type VirtualItem } from "./ChatVirtualList";
import type { MessageKind } from "./heightCache";

type DemoItem = { id: string; kind: MessageKind; height: number };

const makeItems = (count: number, height = 220): DemoItem[] =>
	Array.from({ length: count }, (_, i) => ({
		id: `m-${i}`,
		kind: "assistant" as MessageKind,
		height,
	}));

const StoryHarness: FC<{
	initialCount?: number;
	hasMoreMessages?: boolean;
	itemHeight?: number;
}> = ({ initialCount = 40, hasMoreMessages = false, itemHeight = 220 }) => {
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);
	const scrollToBottomRef = useRef<(() => void) | null>(null);
	const [items, setItems] = useState<DemoItem[]>(() =>
		makeItems(initialCount, itemHeight),
	);
	const [fetchCount, setFetchCount] = useState(0);

	// Derive new ids purely from the previous list so the updater stays pure.
	// React StrictMode double-invokes updaters; a mutable counter would advance
	// twice and make the appended/prepended ids unpredictable.
	const append = () =>
		setItems((prev) => {
			const nextIndex =
				prev.reduce((max, it) => {
					const n = it.id.startsWith("m-") ? Number(it.id.slice(2)) : -1;
					return Number.isNaN(n) ? max : Math.max(max, n);
				}, -1) + 1;
			return [
				...prev,
				{ id: `m-${nextIndex}`, kind: "assistant", height: itemHeight },
			];
		});
	const prepend = () =>
		setItems((prev) => {
			const minOld = prev.reduce((min, it) => {
				const n = it.id.startsWith("old-") ? Number(it.id.slice(4)) : 0;
				return Number.isNaN(n) ? min : Math.min(min, n);
			}, 0);
			const older = Array.from({ length: 5 }, (_, i) => ({
				id: `old-${minOld - 1 - i}`,
				kind: "user" as MessageKind,
				height: 120,
			}));
			return [...older, ...prev];
		});
	const growFive = () =>
		setItems((prev) =>
			prev.map((it) =>
				it.id === "m-5" ? { ...it, height: it.height + 160 } : it,
			),
		);

	const byId = new Map(items.map((it) => [it.id, it]));
	const renderItem = (item: VirtualItem) => {
		const demo = byId.get(item.id);
		return (
			<div data-testid={item.id} style={{ height: demo?.height ?? 120 }}>
				{item.id}
			</div>
		);
	};

	return (
		<div className="flex h-[480px] w-[640px] flex-col">
			<div className="flex gap-1">
				<button type="button" data-testid="append" onClick={append}>
					append
				</button>
				<button type="button" data-testid="prepend" onClick={prepend}>
					prepend
				</button>
				<button type="button" data-testid="grow-5" onClick={growFive}>
					grow-5
				</button>
				<span data-testid="fetch-count">{fetchCount}</span>
			</div>
			<ChatVirtualList
				items={items}
				renderItem={renderItem}
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				isFetchingMoreMessages={false}
				hasMoreMessages={hasMoreMessages}
				onFetchMoreMessages={() => setFetchCount((count) => count + 1)}
			/>
		</div>
	);
};

const meta: Meta<typeof ChatVirtualList> = {
	title: "pages/AgentsPage/ChatVirtualList",
	component: ChatVirtualList,
};
export default meta;

type Story = StoryObj<typeof ChatVirtualList>;

const distanceFromBottom = (scroller: HTMLElement): number =>
	scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;

// visibleAnchorId returns the id of the first rendered item intersecting the
// viewport top, picked dynamically so assertions do not depend on which items
// the window chose to render.
const visibleAnchorId = (
	canvasElement: HTMLElement,
	scroller: HTMLElement,
): string => {
	const top = scroller.getBoundingClientRect().top;
	for (const el of canvasElement.querySelectorAll("[data-chat-item-id]")) {
		if (el.getBoundingClientRect().bottom > top + 5) {
			const id = el.getAttribute("data-chat-item-id");
			if (id) {
				return id;
			}
		}
	}
	throw new Error("no visible item");
};

const offsetOfId = (
	canvasElement: HTMLElement,
	scroller: HTMLElement,
	id: string,
): number => {
	const el = canvasElement.querySelector(`[data-chat-item-id="${id}"]`);
	if (!el) {
		return Number.NaN;
	}
	return el.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
};

// settleWindow waits until the window has re-rendered around the current scroll
// position, i.e. a real item sits at the viewport top. The window recompute is
// async, so picking a reference item before settling races against it and may
// observe a stale window from the previous scroll position.
const settleWindow = async (
	canvasElement: HTMLElement,
	scroller: HTMLElement,
): Promise<void> => {
	await waitFor(() => {
		const id = visibleAnchorId(canvasElement, scroller);
		expect(Math.abs(offsetOfId(canvasElement, scroller, id))).toBeLessThan(400);
	});
};

export const RendersAndOverflows: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight),
		);
	},
};

export const RendersOnlyWindow: Story = {
	render: () => <StoryHarness initialCount={200} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		// Only the near-viewport window is rendered, not all 200 items, and a
		// far-above item is not mounted once the window has converged.
		await waitFor(() => expect(canvas.queryByTestId("m-2")).toBeNull());
		const rendered = canvasElement.querySelectorAll("[data-chat-item]");
		await expect(rendered.length).toBeLessThanOrEqual(40);
		const topSpacer = canvasElement.querySelector("[data-virtual-spacer]");
		await expect((topSpacer as HTMLElement).style.height).not.toBe("0px");
	},
};

export const StaysPinnedWhileAtBottom: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		await userEvent.click(canvas.getByTestId("append"));
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
	},
};

export const DoesNotYankWhenScrolledUp: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		scroller.scrollTop = 800;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(800));
		await userEvent.click(canvas.getByTestId("append"));
		await waitFor(() =>
			expect(Math.abs(scroller.scrollTop - 800)).toBeLessThanOrEqual(2),
		);
	},
};

export const FetchesOlderNearTop: Story = {
	render: () => <StoryHarness hasMoreMessages />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		scroller.scrollTop = 0;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() =>
			expect(
				Number(canvas.getByTestId("fetch-count").textContent),
			).toBeGreaterThan(0),
		);
	},
};

export const ScrollToBottomButtonWorks: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		scroller.scrollTop = 100;
		scroller.dispatchEvent(new Event("scroll"));
		const button = await canvas.findByRole("button", {
			name: "Scroll to bottom",
		});
		await userEvent.click(button);
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
	},
};

export const NoJumpOnPrepend: Story = {
	render: () => <StoryHarness initialCount={200} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		scroller.scrollTop = 20000;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(20000));
		await settleWindow(canvasElement, scroller);
		const ref = visibleAnchorId(canvasElement, scroller);
		const before = offsetOfId(canvasElement, scroller, ref);
		await userEvent.click(canvas.getByTestId("prepend"));
		await waitFor(() =>
			expect(
				Math.abs(offsetOfId(canvasElement, scroller, ref) - before),
			).toBeLessThanOrEqual(3),
		);
	},
};

export const StreamingGrowthAboveViewportNoJump: Story = {
	render: () => <StoryHarness initialCount={200} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		// Put m-5 (offset 1100) into the top overscan, above the viewport top.
		scroller.scrollTop = 1800;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(1800));
		await settleWindow(canvasElement, scroller);
		const ref = visibleAnchorId(canvasElement, scroller);
		const before = offsetOfId(canvasElement, scroller, ref);
		// m-5 grows above the viewport; the visible reference must not move.
		await userEvent.click(canvas.getByTestId("grow-5"));
		await waitFor(() =>
			expect(
				Math.abs(offsetOfId(canvasElement, scroller, ref) - before),
			).toBeLessThanOrEqual(3),
		);
	},
};

export const NoJumpScrollingThroughWrongEstimates: Story = {
	// Real height 120 differs sharply from the assistant seed 220, so items
	// entering the top of the window reconcile estimate to measured. Scrolling up
	// by a delta must move a tracked item by exactly that delta.
	render: () => <StoryHarness initialCount={200} itemHeight={120} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		scroller.scrollTop = 10000;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(10000));
		await settleWindow(canvasElement, scroller);
		const ref = visibleAnchorId(canvasElement, scroller);
		const before = offsetOfId(canvasElement, scroller, ref);
		scroller.scrollTop = 9500;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() =>
			expect(
				Math.abs(offsetOfId(canvasElement, scroller, ref) - (before + 500)),
			).toBeLessThanOrEqual(5),
		);
	},
};

export const OnlyWindowMounts: Story = {
	render: () => <StoryHarness initialCount={1000} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		// The newest item is mounted at the bottom.
		await waitFor(() => expect(canvas.queryByTestId("m-999")).not.toBeNull());
		// DOM node count stays bounded by the window, independent of the 1000-item
		// list size: a render-all implementation would mount far more than this.
		const rendered = canvasElement.querySelectorAll("[data-chat-item]");
		await expect(rendered.length).toBeLessThanOrEqual(40);
	},
};

export const AppendKeepsPinned: Story = {
	render: () => <StoryHarness initialCount={1000} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		await userEvent.click(canvas.getByTestId("append"));
		// The appended item mounts and the scroller stays pinned to the bottom.
		await waitFor(() => expect(canvas.queryByTestId("m-1000")).not.toBeNull());
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
	},
};

export const ScrollRecyclesOffscreen: Story = {
	render: () => <StoryHarness initialCount={200} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await waitFor(() =>
			expect(distanceFromBottom(scroller)).toBeLessThanOrEqual(16),
		);
		// Bring m-100 into the window.
		scroller.scrollTop = 22000;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(canvas.queryByTestId("m-100")).not.toBeNull());
		// Scroll far below m-100 so it falls outside the overscan window.
		scroller.scrollTop = 40000;
		scroller.dispatchEvent(new Event("scroll"));
		// The offscreen row leaves the DOM (true recycling) and the top spacer
		// grows to reserve the space of every item now scrolled above the window,
		// including m-100's slot at offset 22000.
		await waitFor(() => expect(canvas.queryByTestId("m-100")).toBeNull());
		const topSpacer = canvasElement.querySelector("[data-virtual-spacer]");
		await expect(
			Number.parseInt((topSpacer as HTMLElement).style.height, 10),
		).toBeGreaterThan(22000);
	},
};
