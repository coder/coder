import type { Meta, StoryObj } from "@storybook/react-vite";
import { type FC, useRef, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { ChatVirtualList } from "./ChatVirtualList";

type Item = { id: number; height: number };

const StoryHarness: FC<{ hasMoreMessages?: boolean }> = ({
	hasMoreMessages = false,
}) => {
	const scrollContainerRef = useRef<HTMLDivElement | null>(null);
	const scrollToBottomRef = useRef<(() => void) | null>(null);
	const nextId = useRef(40);
	const olderId = useRef(-1);
	const [items, setItems] = useState<Item[]>(() =>
		Array.from({ length: 40 }, (_, i) => ({ id: i, height: 120 })),
	);
	const [fetchCount, setFetchCount] = useState(0);

	const append = () =>
		setItems((prev) => [...prev, { id: nextId.current++, height: 200 }]);
	const prepend = () =>
		setItems((prev) => [
			...Array.from({ length: 5 }, () => ({
				id: olderId.current--,
				height: 120,
			})),
			...prev,
		]);
	const growFive = () =>
		setItems((prev) =>
			prev.map((it) => (it.id === 5 ? { ...it, height: 320 } : it)),
		);

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
				scrollContainerRef={scrollContainerRef}
				scrollToBottomRef={scrollToBottomRef}
				isFetchingMoreMessages={false}
				hasMoreMessages={hasMoreMessages}
				onFetchMoreMessages={() => setFetchCount((count) => count + 1)}
				messageCount={items.length}
			>
				{items.map((it) => (
					<div
						key={it.id}
						data-testid={`msg-${it.id}`}
						style={{ height: it.height }}
					>
						message {it.id}
					</div>
				))}
			</ChatVirtualList>
		</div>
	);
};

const meta: Meta<typeof ChatVirtualList> = {
	title: "pages/AgentsPage/ChatVirtualList",
	component: ChatVirtualList,
};
export default meta;

type Story = StoryObj<typeof ChatVirtualList>;

const offsetFromScrollerTop = (
	scroller: HTMLElement,
	el: HTMLElement,
): number =>
	el.getBoundingClientRect().top - scroller.getBoundingClientRect().top;

const distanceFromBottom = (scroller: HTMLElement): number =>
	scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight;

export const RendersAndOverflows: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		await expect(scroller.scrollHeight).toBeGreaterThan(scroller.clientHeight);
	},
};

export const AnchorStaysStableOnAboveViewportGrowth: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		scroller.scrollTop = 1500;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(1500));
		const before = offsetFromScrollerTop(
			scroller,
			canvas.getByTestId("msg-15"),
		);
		await userEvent.click(canvas.getByTestId("grow-5"));
		await waitFor(() =>
			expect(
				Math.abs(
					offsetFromScrollerTop(scroller, canvas.getByTestId("msg-15")) -
						before,
				),
			).toBeLessThanOrEqual(2),
		);
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
		scroller.scrollTop = 0;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() =>
			expect(
				Number(canvas.getByTestId("fetch-count").textContent),
			).toBeGreaterThan(0),
		);
	},
};

export const PrependKeepsViewportStable: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
		scroller.scrollTop = 1500;
		scroller.dispatchEvent(new Event("scroll"));
		await waitFor(() => expect(scroller.scrollTop).toBe(1500));
		const before = offsetFromScrollerTop(
			scroller,
			canvas.getByTestId("msg-20"),
		);
		await userEvent.click(canvas.getByTestId("prepend"));
		await waitFor(() =>
			expect(
				Math.abs(
					offsetFromScrollerTop(scroller, canvas.getByTestId("msg-20")) -
						before,
				),
			).toBeLessThanOrEqual(2),
		);
	},
};

export const ScrollToBottomButtonWorks: Story = {
	render: () => <StoryHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const scroller = canvas.getByTestId("scroll-container");
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
