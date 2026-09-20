import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, userEvent } from "storybook/test";
import {
	FIXTURE_NOW,
	MockWorkingBlock,
	pinFixtureClock,
} from "./storyFixtures";
import { WorkingBlockDisclosure } from "./WorkingBlockDisclosure";

const meta = {
	title: "pages/AgentsPage/ChatConversation/WorkingBlockDisclosure",
	component: WorkingBlockDisclosure,
	beforeEach: pinFixtureClock,
	args: {
		block: MockWorkingBlock,
		expanded: false,
		onExpandedChange: fn(),
		children: (
			<ul aria-label="Steps">
				<li>Read src/main.ts</li>
				<li>Ran project tests</li>
			</ul>
		),
	},
} satisfies Meta<typeof WorkingBlockDisclosure>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Collapsed: Story = {};

export const SummaryFocused: Story = {
	play: async () => {
		await userEvent.tab();
	},
};

export const Expanded: Story = {
	args: { expanded: true },
};

export const ShortSingleStep: Story = {
	args: {
		block: { ...MockWorkingBlock, stepCount: 1, startedAt: FIXTURE_NOW - 999 },
	},
};

export const LongDuration: Story = {
	args: {
		block: { ...MockWorkingBlock, startedAt: FIXTURE_NOW - 3_785_000 },
	},
};

export const UnknownDuration: Story = {
	args: {
		block: { ...MockWorkingBlock, startedAt: undefined, endedAt: undefined },
	},
};

export const FailedSteps: Story = {
	args: { block: { ...MockWorkingBlock, failedCount: 1 } },
};

export const Live: Story = {
	args: { block: { ...MockWorkingBlock, isLive: true, endedAt: undefined } },
};

export const LiveWithoutTimestamp: Story = {
	args: {
		block: {
			...MockWorkingBlock,
			isLive: true,
			startedAt: undefined,
			endedAt: undefined,
		},
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
