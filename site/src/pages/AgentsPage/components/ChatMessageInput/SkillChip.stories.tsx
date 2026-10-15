import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { EditableSkillChip, SkillChip } from "./SkillChip";

const meta: Meta<typeof SkillChip> = {
	title: "components/ChatMessageInput/SkillChip",
	component: SkillChip,
	args: {
		source: "project",
		name: "triage",
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
type Story = StoryObj<typeof SkillChip>;

export const InlineWithText: Story = {
	render: (args) => (
		<p className="m-0 font-sans text-sm leading-6 text-content-primary">
			Use <SkillChip {...args} /> on the flaky test.
		</p>
	),
};

export const LongNameTruncates: Story = {
	args: {
		name: "summarize-every-failing-integration-test-in-the-workspace",
	},
};

export const Editable: Story = {
	render: (args) => (
		<EditableSkillChip {...args} selected={false} onRemove={fn()} />
	),
};

export const EditableSelected: Story = {
	render: (args) => <EditableSkillChip {...args} selected onRemove={fn()} />,
};
