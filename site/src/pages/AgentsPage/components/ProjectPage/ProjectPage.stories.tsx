import type { Meta, StoryObj } from "@storybook/react-vite";
import { spyOn } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	chatProjectACLKey,
	chatProjectExternalAuthKey,
	chatProjectInstructionsKey,
	chatProjectRunsKey,
	chatProjectsKey,
	chatProjectTriggersKey,
} from "#/api/queries/chatProjects";
import {
	chatCostTreeKey,
	mcpServerConfigsKey,
	projectChatsKey,
} from "#/api/queries/chats";
import { externalAuthsKey } from "#/api/queries/externalAuth";
import { getTemplatesQueryKey } from "#/api/queries/templates";
import { userKey } from "#/api/queries/users";
import type { Chat } from "#/api/typesGenerated";
import {
	MockChat,
	MockMCPServerConfig,
	mockChatCost,
} from "#/testHelpers/chatEntities";
import {
	MockChatProject,
	MockChatProjectACL,
	MockChatProjectExternalAuth,
	MockChatProjectInstructions,
	MockChatProjectRuns,
	MockChatProjectSharedUse,
	MockChatProjectTrigger,
	MockChatSecurityAgent,
	MockTemplate,
	MockUserMember,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { ProjectPage } from "./ProjectPage";

const chatQueries = (chats: Chat[]) => [
	{
		key: projectChatsKey(MockChatProject.id),
		data: { pages: [chats], pageParams: [0] },
	},
	...chats.map((chat) => ({
		key: chatCostTreeKey(chat.id),
		data: mockChatCost(chat.id, 1_234_000),
	})),
];

const tabRouter = (projectId: string, tab?: string) =>
	reactRouterParameters({
		location: {
			path: tab
				? `/agents/projects/${projectId}/${tab}`
				: `/agents/projects/${projectId}`,
			pathParams: tab ? { projectId, tab } : { projectId },
		},
		routing: { path: "/agents/projects/:projectId/:tab?" },
	});

const aclQuery = {
	key: chatProjectACLKey(MockChatProject.id),
	data: MockChatProjectACL,
};

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
		features: ["aibridge"],
		queries: [
			{
				key: chatProjectExternalAuthKey(MockChatProject.id),
				data: MockChatProjectExternalAuth,
			},
			aclQuery,
			{ key: chatProjectsKey, data: [MockChatProject, MockChatSecurityAgent] },
			{
				key: chatProjectRunsKey(MockChatProject.id),
				data: MockChatProjectRuns,
			},
			{
				key: chatProjectTriggersKey(MockChatProject.id),
				data: [MockChatProjectTrigger],
			},
			{ key: getTemplatesQueryKey(), data: [MockTemplate] },
			{
				key: mcpServerConfigsKey(MockChatProject.organization_id),
				data: [MockMCPServerConfig],
			},
			{ key: externalAuthsKey, data: { providers: [], links: [] } },
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
		],
	},
};

export default meta;
type Story = StoryObj<typeof ProjectPage>;

/** Each story's queries add to the defaults above; later keys win. */
const withQueries = (
	...queries: { key: readonly unknown[]; data: unknown }[]
) => [...(meta.parameters?.queries ?? []), ...queries];

export const Desktop: Story = {};

/** Enough chats to overflow a phone screen, so the page scrolls. */
export const Mobile: Story = {
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
		queries: withQueries(
			...chatQueries(
				Array.from({ length: 12 }, (_, index) => ({
					...MockChat,
					id: `chat-${index}`,
					title: `Chat ${index + 1}`,
				})),
			),
		),
	},
};

/** Another user's project in a multi-organization deployment. */
export const OwnedByAnotherUser: Story = {
	args: {
		project: {
			...MockChatProject,
			owner_id: MockUserMember.id,
			permissions: MockChatProjectSharedUse.permissions,
		},
	},
	parameters: {
		showOrganizations: true,
		queries: withQueries({
			key: userKey(MockUserMember.id),
			data: MockUserMember,
		}),
	},
};

export const RunHistory: Story = {
	parameters: { reactRouter: tabRouter(MockChatProject.id, "run-history") },
};

export const Settings: Story = {
	args: {
		project: {
			...MockChatProject,
			invokable_agent_ids: [MockChatSecurityAgent.id],
		},
	},
	parameters: { reactRouter: tabRouter(MockChatProject.id, "settings") },
};

/** An agent shared with you shows its settings without edit controls. */
export const SettingsReadOnly: Story = {
	args: { project: MockChatProjectSharedUse },
	parameters: {
		showOrganizations: true,
		reactRouter: tabRouter(MockChatProjectSharedUse.id, "settings"),
		queries: withQueries(
			{
				key: chatProjectInstructionsKey(MockChatProjectSharedUse.id),
				data: MockChatProjectInstructions,
			},
			{ key: userKey(MockUserMember.id), data: MockUserMember },
		),
	},
};

const withoutSharingQuery = (meta.parameters?.queries ?? []).filter(
	(query: { key: readonly unknown[] }) => query !== aclQuery,
);

export const SharingLoading: Story = {
	parameters: { queries: withoutSharingQuery },
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectACL").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const SharingLoadError: Story = {
	parameters: { queries: withoutSharingQuery },
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectACL").mockRejectedValue(
			mockApiError({ message: "Failed to load sharing." }),
		);
	},
};
