import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { DiffStatsInline } from "./DiffStats";

const meta: Meta<typeof DiffStatsInline> = {
	title: "pages/AgentsPage/DiffStatsInline",
	component: DiffStatsInline,
	args: {
		status: {
			chat_id: "chat-1",
			changes_requested: false,
			additions: 42,
			deletions: 7,
			changed_files: 5,
		},
		onClick: fn(),
	},
};
export default meta;
type Story = StoryObj<typeof DiffStatsInline>;

export const Default: Story = {};

export const ZeroLinesWithFiles: Story = {
	args: {
		status: {
			chat_id: "chat-2",
			changes_requested: false,
			additions: 0,
			deletions: 0,
			changed_files: 3,
		},
	},
};

export const NoChanges: Story = {
	args: {
		status: {
			chat_id: "chat-3",
			changes_requested: false,
			additions: 0,
			deletions: 0,
			changed_files: 0,
		},
	},
};

export const LargeNumbers: Story = {
	args: {
		status: {
			chat_id: "chat-4",
			changes_requested: false,
			additions: 1234,
			deletions: 567,
			changed_files: 42,
		},
	},
};
