import type { Meta, StoryObj } from "@storybook/react-vite";
import { DiffStatBadge } from "./DiffStats";

const badgeMeta: Meta<typeof DiffStatBadge> = {
	title: "pages/AgentsPage/DiffStatBadge",
	component: DiffStatBadge,
	decorators: [
		(Story) => (
			<div className="flex h-6 items-center">
				<Story />
			</div>
		),
	],
	args: {
		additions: 42,
		deletions: 7,
	},
};
export default badgeMeta;
type BadgeStory = StoryObj<typeof DiffStatBadge>;

export const Default: BadgeStory = {};

export const AdditionsOnly: BadgeStory = {
	args: { additions: 10, deletions: 0 },
};

export const DeletionsOnly: BadgeStory = {
	args: { additions: 0, deletions: 5 },
};

export const ZeroChanges: BadgeStory = {
	args: { additions: 0, deletions: 0 },
};

export const LargeNumbers: BadgeStory = {
	args: { additions: 1234, deletions: 567 },
};
