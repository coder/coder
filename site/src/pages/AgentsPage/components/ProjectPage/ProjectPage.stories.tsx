import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { chatProjectSkillsKey } from "#/api/queries/chatProjectSkills";
import {
	chatProjectACLKey,
	chatProjectInstructionsKey,
} from "#/api/queries/chatProjects";
import {
	archivedProjectChatsKey,
	chatCostTreeKey,
	projectChatsKey,
} from "#/api/queries/chats";
import type { Chat, ChatProjectACL } from "#/api/typesGenerated";
import { MockChat, mockChatCost } from "#/testHelpers/chatEntities";
import {
	MockArchivedChatProject,
	MockChatProject,
	MockChatProjectACL,
	MockChatProjectInstructions,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { MockSkills } from "#/testHelpers/skills";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ProjectPage } from "./ProjectPage";

const chatQueries = (chats: Chat[]) => [
	{
		key: projectChatsKey(MockChatProject.id),
		data: {
			pages: [
				chats.map((chat) => ({ ...chat, project_id: MockChatProject.id })),
			],
			pageParams: [0],
		},
	},
	...chats.map((chat) => ({
		key: chatCostTreeKey(chat.id),
		data: mockChatCost(chat.id, 1_234_000),
	})),
];

const mockACL: ChatProjectACL = { ...MockChatProjectACL, groups: [] };

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
		isOwnedOrSharedWithUser: true,
		children: (
			<div className="flex h-32 items-center justify-center rounded-lg border border-dashed border-border text-sm text-content-secondary">
				Composer
			</div>
		),
	},
	parameters: {
		user: MockUserOwner,
		features: ["aibridge"],
		queries: [
			...chatQueries([
				{ ...MockChat, id: "chat-1", title: "Plan the launch" },
				{
					...MockChat,
					id: "chat-2",
					title: "Draft the announcement",
					status: "running",
				},
			]),
			{
				key: chatProjectInstructionsKey(MockChatProject.id),
				data: MockChatProjectInstructions,
			},
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: MockSkills,
			},
			{ key: chatProjectACLKey(MockChatProject.id), data: mockACL },
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

export const Desktop: Story = {};

/** Enough chats to overflow a phone screen, so the page scrolls. */
export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
		queries: [
			...chatQueries(
				Array.from({ length: 12 }, (_, index) => ({
					...MockChat,
					id: `chat-${index}`,
					title: `Chat ${index + 1}`,
				})),
			),
			{
				key: chatProjectInstructionsKey(MockChatProject.id),
				data: MockChatProjectInstructions,
			},
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: MockSkills,
			},
			{ key: chatProjectACLKey(MockChatProject.id), data: mockACL },
		],
	},
};

/** An admin can manage the project but can't start chats in it. */
export const NotSharedWithUser: Story = {
	args: {
		project: { ...MockChatProject, owner_id: MockUserMember.id },
		isOwnedOrSharedWithUser: false,
	},
	parameters: {
		queries: [
			...chatQueries([]),
			{
				key: chatProjectInstructionsKey(MockChatProject.id),
				data: MockChatProjectInstructions,
			},
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: MockSkills,
			},
			{
				key: chatProjectACLKey(MockChatProject.id),
				data: { owner: MockUserMember, users: [], groups: [] },
			},
		],
	},
};

/** Another user's project in a multi-organization deployment. */
export const OwnedByAnotherUser: Story = {
	args: { project: { ...MockChatProject, owner_id: MockUserMember.id } },
	parameters: {
		showOrganizations: true,
		queries: [
			...chatQueries([{ ...MockChat, id: "chat-1", title: "Plan the launch" }]),
			{
				key: chatProjectInstructionsKey(MockChatProject.id),
				data: MockChatProjectInstructions,
			},
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: MockSkills,
			},
			{
				key: chatProjectACLKey(MockChatProject.id),
				data: {
					owner: MockUserMember,
					users: [{ ...MockUserOwner, role: "admin" }],
					groups: [],
				},
			},
		],
	},
};

export const WithoutUpdateOrDeletePermission: Story = {
	args: {
		project: {
			...MockChatProject,
			permissions: {
				update: false,
				delete: false,
				share: false,
				archive: false,
			},
		},
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await canvas.findByText(/Use TypeScript for new code/);
		expect(
			canvas.queryByRole("button", { name: "Edit instructions" }),
		).not.toBeInTheDocument();
	},
};

export const WithoutDeletePermission: Story = {
	args: {
		project: {
			...MockChatProject,
			permissions: { update: true, delete: false, share: true, archive: false },
		},
	},
};

/** A chat that moved to another project leaves the list before the refetch. */
export const ChatMovedToAnotherProject: Story = {
	parameters: {
		queries: [
			{
				key: projectChatsKey(MockChatProject.id),
				data: {
					pages: [
						[
							{
								...MockChat,
								id: "chat-1",
								title: "Plan the launch",
								project_id: MockChatProject.id,
							},
							{
								...MockChat,
								id: "chat-2",
								title: "Moved to another project",
								project_id: "billing",
							},
						],
					],
					pageParams: [0],
				},
			},
			{
				key: chatCostTreeKey("chat-1"),
				data: mockChatCost("chat-1", 1_234_000),
			},
			{
				key: chatProjectInstructionsKey(MockChatProject.id),
				data: MockChatProjectInstructions,
			},
			{
				key: chatProjectSkillsKey(MockChatProject.id),
				data: MockSkills,
			},
			{ key: chatProjectACLKey(MockChatProject.id), data: mockACL },
		],
	},
};

const archivedQueries = [
	{
		key: archivedProjectChatsKey(MockChatProject.id),
		data: {
			pages: [
				[
					{
						...MockChat,
						id: "chat-1",
						title: "Plan the launch",
						archived: true,
						project_id: MockChatProject.id,
					},
				],
			],
			pageParams: [0],
		},
	},
	{
		key: chatCostTreeKey("chat-1"),
		data: mockChatCost("chat-1", 1_234_000),
	},
	{
		key: chatProjectInstructionsKey(MockChatProject.id),
		data: MockChatProjectInstructions,
	},
	{
		key: chatProjectSkillsKey(MockChatProject.id),
		data: MockSkills,
	},
	{ key: chatProjectACLKey(MockChatProject.id), data: mockACL },
];

/** The composer gives way to a banner, and the list shows archived chats. */
export const Archived: Story = {
	args: { project: MockArchivedChatProject },
	parameters: { queries: archivedQueries },
};

/** Only Unarchive project and Delete project remain in the menu. */
export const ArchivedProjectActions: Story = {
	args: { project: MockArchivedChatProject },
	parameters: { queries: archivedQueries },
	play: async ({ canvasElement }) => {
		await userEvent.click(
			await within(canvasElement).findByRole("button", {
				name: "Project actions",
			}),
		);
	},
};

/** A user who can't unarchive the project gets no Unarchive button. */
export const ArchivedWithoutArchivePermission: Story = {
	args: {
		project: {
			...MockArchivedChatProject,
			permissions: {
				update: false,
				delete: false,
				share: false,
				archive: false,
			},
		},
	},
	parameters: { queries: archivedQueries },
};
