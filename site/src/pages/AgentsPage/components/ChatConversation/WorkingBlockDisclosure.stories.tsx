import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ComponentProps, useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { FIXTURE_NOW } from "./storyFixtures";
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";
import type { WorkingBlock } from "./workingBlockGrouping";

const MockWorkingBlock: WorkingBlock = {
	key: "working:through:message:5",
	liveKey: "working:live:message:1:0",
	rowIndices: [0, 1],
	memberIds: [2, 4],
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

export const Collapsed: Story = {};

export const KeyboardToggle: Story = {
	play: async () => {
		await userEvent.tab();
		await userEvent.keyboard("{Enter}");
		await userEvent.keyboard(" ");
	},
};

export const Expanded: Story = {
	args: { expanded: true },
};

export const ShortSingleStep: Story = {
	args: {
		block: { ...MockWorkingBlock, stepCount: 1, endedAt: FIXTURE_NOW + 999 },
	},
};

export const LongDuration: Story = {
	args: { block: { ...MockWorkingBlock, endedAt: FIXTURE_NOW + 3_785_000 } },
};

export const UnknownDuration: Story = {
	args: {
		block: { ...MockWorkingBlock, startedAt: undefined, endedAt: undefined },
	},
};

export const FailedSteps: Story = {
	args: { block: { ...MockWorkingBlock, failedCount: 1 } },
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
		await userEvent.click(
			canvas.getByRole("button", { name: "Working for 12s" }),
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Advance one second" }),
		);
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
};

export const PartialHistory: Story = {
	args: { block: { ...MockWorkingBlock, isPartial: true } },
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
};
