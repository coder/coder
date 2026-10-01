import type { Meta, StoryObj } from "@storybook/react-vite";
import { projectChatsKey } from "#/api/queries/chats";
import { MockChat } from "#/testHelpers/chatEntities";
import { MockChatProject, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ProjectPage } from "./ProjectPage";

const meta: Meta<typeof ProjectPage> = {
	title: "pages/AgentsPage/ProjectPage/ProjectPage",
	component: ProjectPage,
	decorators: [
		(Story) => (
			<div className="flex h-screen flex-col">
				<Story />
			</div>
		),
		withAuthProvider,
		withDashboardProvider,
	],
	args: {
		project: MockChatProject,
		children: (
			<div className="flex h-32 items-center justify-center rounded-lg border border-dashed border-border text-sm text-content-secondary">
				Composer
			</div>
		),
	},
	parameters: {
		user: MockUserOwner,
		queries: [
			{
				key: projectChatsKey(MockChatProject.id),
				data: {
					pages: [
						[
							{ ...MockChat, id: "chat-1", title: "Plan the launch" },
							{
								...MockChat,
								id: "chat-2",
								title: "Draft the announcement",
								status: "running",
							},
						],
					],
					pageParams: [0],
				},
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

export const Desktop: Story = {};

export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
};
