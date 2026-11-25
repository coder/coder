import type { Meta, StoryObj } from "@storybook/react-vite";
import { EllipsisVerticalIcon } from "lucide-react";
import { Button } from "#/components/Button/Button";
import { MockChatProject } from "#/testHelpers/entities";
import { ProjectPageHeader } from "./ProjectPageHeader";

const actions = (
	<Button variant="subtle" size="icon" aria-label="Project actions">
		<EllipsisVerticalIcon />
	</Button>
);

const meta: Meta<typeof ProjectPageHeader> = {
	title: "pages/AgentsPage/ProjectPage/ProjectPageHeader",
	component: ProjectPageHeader,
	decorators: [
		(Story) => (
			<div className="max-w-5xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		project: MockChatProject,
		actions,
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPageHeader>;

export const Default: Story = {};

export const WithoutDescription: Story = {
	args: {
		project: { ...MockChatProject, description: "" },
	},
};

export const LongNameAndDescription: Story = {
	args: {
		project: {
			...MockChatProject,
			name: "N".repeat(64),
			description: "d".repeat(1024),
		},
	},
};
