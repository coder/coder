import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { FIXTURE_NOW } from "./storyFixtures";
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";
import type { WorkingBlock } from "./workingBlockGrouping";

const MockWorkingBlock: WorkingBlock = {
	key: "working:through:message:5",
	liveKey: "working:live:message:1:0",
	rowIndices: [0, 1],
	startedAt: FIXTURE_NOW,
	endedAt: FIXTURE_NOW + 12_000,
	stepCount: 2,
	failedCount: 0,
	isLive: false,
	isPartial: false,
};

const ControlledDisclosure = (
	props: ComponentProps<typeof WorkingBlockDisclosure>,
) => {
	const [expanded, setExpanded] = useState(props.expanded);
	return (
		<WorkingBlockDisclosure
			{...props}
			expanded={expanded}
			onExpandedChange={(next) => {
				setExpanded(next);
				props.onExpandedChange(next);
			}}
		/>
	);
};

const meta = {
	title: "pages/AgentsPage/ChatConversation/WorkingBlockDisclosure",
	component: WorkingBlockDisclosure,
	render: (args) => <ControlledDisclosure {...args} />,
	args: {
		block: MockWorkingBlock,
		rowKeys: ["message:2", "message:4"],
		expanded: false,
		onExpandedChange: fn(),
		children: (
			<ul aria-label="Original tool steps">
				<li>Read src/main.ts</li>
				<li>Ran project tests</li>
			</ul>
		),
	},
} satisfies Meta<typeof WorkingBlockDisclosure>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Collapsed: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Worked for 12s (2 steps)" }),
		).toHaveAttribute("aria-expanded", "false");
		await expect(
			canvas.queryByRole("list", { name: "Original tool steps" }),
		).not.toBeInTheDocument();
	},
};

export const KeyboardToggle: Story = {
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		await userEvent.tab();
		await expect(trigger).toHaveFocus();
		await userEvent.keyboard("{Enter}");
		await expect(trigger).toHaveAttribute("aria-expanded", "true");
		await expect(
			canvas.getByRole("list", { name: "Original tool steps" }),
		).toBeVisible();
		await expect(args.onExpandedChange).toHaveBeenLastCalledWith(true);
		await userEvent.keyboard(" ");
		await expect(trigger).toHaveAttribute("aria-expanded", "false");
		await expect(
			canvas.queryByRole("list", { name: "Original tool steps" }),
		).not.toBeInTheDocument();
		await expect(args.onExpandedChange).toHaveBeenLastCalledWith(false);
		await expect(trigger).toHaveFocus();
	},
};

export const Expanded: Story = {
	args: { expanded: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Worked for 12s (2 steps)",
		});
		await expect(
			canvas.getByRole("list", { name: "Original tool steps" }),
		).toBeVisible();
		await userEvent.click(trigger);
		await expect(trigger).toHaveAttribute("aria-expanded", "false");
		await userEvent.click(trigger);
		await expect(
			canvas.getByRole("list", { name: "Original tool steps" }),
		).toBeVisible();
	},
};

export const ShortSingleStep: Story = {
	args: {
		block: { ...MockWorkingBlock, stepCount: 1, endedAt: FIXTURE_NOW + 999 },
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Worked for 0s (1 step)",
			}),
		).toBeVisible();
	},
};

export const LongDuration: Story = {
	args: { block: { ...MockWorkingBlock, endedAt: FIXTURE_NOW + 3_785_000 } },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Worked for 1h 3m (2 steps)",
			}),
		).toBeVisible();
	},
};

export const UnknownDuration: Story = {
	args: {
		block: { ...MockWorkingBlock, startedAt: undefined, endedAt: undefined },
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Completed 2 steps",
			}),
		).toBeVisible();
	},
};

export const FailedSteps: Story = {
	args: { block: { ...MockWorkingBlock, failedCount: 1 } },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Worked for 12s (2 steps) 1 failed step",
			}),
		).toBeVisible();
	},
};

const LiveClock = (args: ComponentProps<typeof WorkingBlockDisclosure>) => {
	const [now, setNow] = useState(FIXTURE_NOW + 12_000);
	return (
		<>
			<ControlledDisclosure {...args} now={now} />
			<Button onClick={() => setNow((current) => current + 1000)}>
				Advance one second
			</Button>
		</>
	);
};

export const LiveTimer: Story = {
	args: { block: { ...MockWorkingBlock, isLive: true, endedAt: undefined } },
	render: (args) => <LiveClock {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", { name: "Working for 12s" });
		await userEvent.click(trigger);
		await expect(
			canvas.getByRole("list", { name: "Original tool steps" }),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance one second" }),
		);
		await expect(
			canvas.getByRole("button", { name: "Working for 13s" }),
		).toHaveAttribute("aria-expanded", "true");
		await expect(
			canvas.getByRole("list", { name: "Original tool steps" }),
		).toBeVisible();
	},
};

export const LiveWithoutTimestamp: Story = {
	args: {
		block: {
			...MockWorkingBlock,
			isLive: true,
			startedAt: undefined,
			endedAt: undefined,
		},
		now: FIXTURE_NOW,
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", { name: "Working" }),
		).toBeVisible();
	},
};

export const PartialLive: Story = {
	args: {
		block: {
			...MockWorkingBlock,
			isLive: true,
			isPartial: true,
			endedAt: undefined,
		},
		now: FIXTURE_NOW + 12_000,
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Working for at least 12s",
			}),
		).toBeVisible();
	},
};

export const PartialHistory: Story = {
	args: { block: { ...MockWorkingBlock, isPartial: true } },
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Worked for at least 12s (2 steps or more)",
			}),
		).toBeVisible();
	},
};

// Unloaded history may hold more failures, so the badge reads as a lower
// bound like the step count.
export const PartialHistoryWithFailedSteps: Story = {
	args: { block: { ...MockWorkingBlock, isPartial: true, failedCount: 1 } },
};

export const PartialHistoryWithoutTimestamps: Story = {
	args: {
		block: {
			...MockWorkingBlock,
			isPartial: true,
			startedAt: undefined,
			endedAt: undefined,
		},
	},
	play: async ({ canvasElement }) => {
		await expect(
			within(canvasElement).getByRole("button", {
				name: "Completed 2 steps or more",
			}),
		).toBeVisible();
	},
};
