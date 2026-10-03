import type { Meta, StoryObj } from "@storybook/react-vite";
import { SectionHeader } from "./SectionHeader";

const meta: Meta<typeof SectionHeader> = {
	title: "pages/AgentsPage/SectionHeader",
	component: SectionHeader,
	args: {
		label: "Agent Configuration",
	},
	decorators: [
		(Story) => (
			<div style={{ maxWidth: 600 }}>
				<Story />
			</div>
		),
	],
};
export default meta;
type Story = StoryObj<typeof SectionHeader>;

export const Default: Story = {};

export const WithDescription: Story = {
	args: {
		label: "Model Settings",
		description: "Configure which AI models are available for agents.",
	},
};

export const WithAction: Story = {
	args: {
		label: "Notifications",
		description: "Manage how you receive notifications.",
		action: <button type="button">Edit</button>,
	},
};

export const LabelOnly: Story = {
	args: {
		label: "Advanced",
		description: undefined,
		action: undefined,
	},
};
