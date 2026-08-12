import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	EditableFileReferenceChip,
	FileReferenceChip,
} from "./FileReferenceChip";

const meta: Meta<typeof FileReferenceChip> = {
	title: "components/ChatMessageInput/FileReferenceChip",
	component: FileReferenceChip,
	args: {
		fileName: "site/src/components/Button.tsx",
		startLine: 42,
		endLine: 42,
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
		selected: true,
	},
};

export const InlineWithText: Story = {
	render: (args) => (
		<p className="m-0 font-sans text-sm leading-6 text-content-primary">
			Can you refactor <FileReferenceChip {...args} /> to use the new API?
		</p>
	),
};

export const LeftAlignedInline: Story = {
	render: (args) => (
		<p className="m-0 font-sans text-sm leading-6 text-content-primary">
			<FileReferenceChip {...args} /> starts this message.
		</p>
	),
};

export const AbuttingInlineText: Story = {
	render: (args) => (
		<p className="m-0 font-sans text-sm leading-6 text-content-primary">
			<span>Before</span>
			<FileReferenceChip {...args} className="ml-1 mr-1" />
			<span>after</span>
		</p>
	),
};

export const Editable: StoryObj<typeof EditableFileReferenceChip> = {
	render: (args) => <EditableFileReferenceChip {...args} />,
	args: {
		fileName: "site/src/components/Button.tsx",
		startLine: 42,
		endLine: 42,
		onOpen: fn(),
		onRemove: fn(),
		selected: true,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = canvas.getByRole("button", {
			name: "Open site/src/components/Button.tsx:L42",
		});
		const removeButton = canvas.getByRole("button", {
			name: "Remove reference",
		});

		await userEvent.click(trigger);
		expect(args.onOpen).toHaveBeenCalledTimes(1);

		await userEvent.click(removeButton);
		expect(args.onRemove).toHaveBeenCalledTimes(1);
		expect(args.onOpen).toHaveBeenCalledTimes(1);
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
