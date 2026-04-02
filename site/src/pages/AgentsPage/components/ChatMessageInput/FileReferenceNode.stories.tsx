import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { FileReferenceChip } from "./FileReferenceNode";

const meta: Meta<typeof FileReferenceChip> = {
	title: "components/ChatMessageInput/FileReferenceChip",
	component: FileReferenceChip,
	args: {
		fileName: "site/src/components/Button.tsx",
		startLine: 42,
		endLine: 42,
		onRemove: fn(),
		onClick: fn(),
	},
	decorators: [
		(Story) => (
			<div style={{ padding: 24 }}>
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof FileReferenceChip>;

export const Default: Story = {};

export const LineRange: Story = {
	args: {
		startLine: 10,
		endLine: 50,
	},
};

export const Selected: Story = {
	args: {
		isSelected: true,
	},
};

export const WithoutRemove: Story = {
	args: {
		onRemove: undefined,
	},
};

/** Chip with a long filename that exceeds the max-width and truncates
 * from the start, keeping the most distinctive part of the name visible. */
export const LongFileNameTruncation: Story = {
	args: {
		fileName:
			"site/src/pages/AgentsPage/components/UserCompactionThresholdSettings.tsx",
		startLine: 274,
		endLine: 289,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chip = canvas.getByTitle(/UserCompactionThresholdSettings/);
		// The chip should be constrained to its max-width and not overflow.
		expect(chip.scrollWidth).toBeLessThanOrEqual(300);
	},
};
