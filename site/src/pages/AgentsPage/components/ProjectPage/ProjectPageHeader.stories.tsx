import type { Meta, StoryObj } from "@storybook/react-vite";
import { EllipsisVerticalIcon, PlusIcon } from "lucide-react";
import { userEvent, within } from "storybook/test";
import { Button } from "#/components/Button/Button";
import { MockChatProject, MockUserOwner } from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import { AvatarGroup } from "./AvatarGroup";
import { ProjectPageHeader } from "./ProjectPageHeader";

const sharing = (
	<div className="flex items-center gap-3">
		<AvatarGroup
			items={[
				{ id: "ada", name: "Ada" },
				{ id: "grace", name: "Grace" },
			]}
			label="Shared with 2 users and groups"
		/>
		<Button variant="outline" size="sm">
			<PlusIcon />
			Share
		</Button>
	</div>
);

const actions = (
	<Button variant="subtle" size="icon" aria-label="Agent actions">
		<EllipsisVerticalIcon />
	</Button>
);

const meta: Meta<typeof ProjectPageHeader> = {
	title: "pages/AgentsPage/ProjectPage/ProjectPageHeader",
	component: ProjectPageHeader,
	parameters: { user: MockUserOwner },
	decorators: [
		withAuthProvider,
		(Story) => (
			<div className="max-w-3xl p-6">
				<Story />
			</div>
		),
	],
	args: {
		project: MockChatProject,
		ownerLabel: "Admin User",
		isOwnedByOtherUser: false,
		onSaveName: async () => undefined,
		sharing,
		actions,
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPageHeader>;

export const Default: Story = {};

export const SharedWithYou: Story = {
	args: {
		isOwnedByOtherUser: true,
		onSaveName: undefined,
		sharing: undefined,
		actions: undefined,
	},
};

export const OwnerLoading: Story = { args: { ownerLabel: undefined } };

export const WithOrganization: Story = {
	args: { organizationLabel: "Coder" },
};

export const WithoutAgentUser: Story = {
	args: { project: { ...MockChatProject, agent_user: undefined } },
};

export const WithoutDescription: Story = {
	args: { project: { ...MockChatProject, description: "" } },
};

export const EditingName: Story = {
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Edit Name" }),
		);
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
