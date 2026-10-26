import type { Meta, StoryObj } from "@storybook/react-vite";
import { Outlet } from "react-router";
import { spyOn, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { getAuthorizationKey } from "#/api/queries/authCheck";
import {
	chatAutomationSchedulePreviewKey,
	chatAutomationsKey,
} from "#/api/queries/chatAutomations";
import { chatEntityKey, organizationChatModelsKey } from "#/api/queries/chats";
import type {
	Chat,
	ChatAutomation,
	ChatModel,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import { MockDefaultOrganization, MockUserOwner } from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import AgentAutomationEditorPage from "./AgentAutomationEditorPage";
import type { AgentsPageOutletContext } from "./AgentsPageLayout";
import { selectedOrganizationIdStorageKey } from "./components/AgentCreateForm";

const organizationId = MockDefaultOrganization.id;
const storyTimeZone = "UTC";

const mockModel: ChatModel = {
	...MockChatModel,
	organization_id: organizationId,
	display_name: "Acme Reasoner 2",
};

const mockTargetChat: Chat = {
	...MockChat,
	organization_id: organizationId,
	title: "CI investigations",
	last_model_config_id: mockModel.id,
};

const heartbeat: ChatAutomation = {
	...MockChatAutomation,
	organization_id: organizationId,
	owner_id: MockUserOwner.id,
	name: "CI heartbeat",
	prompt: "Check the CI run and report progress here.",
	schedule_cron: "*/5 * * * *",
	schedule_time_zone: storyTimeZone,
	target_chat_id: mockTargetChat.id,
	target_chat: {
		id: mockTargetChat.id,
		title: mockTargetChat.title,
		archived: false,
	},
	when_busy: "skip",
};

const othersWebhook: ChatAutomation = {
	...MockWebhookChatAutomation,
	organization_id: organizationId,
	owner_id: "another-user-id",
	owner: {
		id: "another-user-id",
		username: "bb-seed",
		name: "Bug bash bb-seed",
	},
	name: "Seed CI webhook",
	prompt: "Inspect this CI failure. Summarize the likely cause.",
};

const nextRunTimes = (cron: string) =>
	cron === "*/5 * * * *"
		? ["2026-10-12T09:20:00Z", "2026-10-12T09:25:00Z", "2026-10-12T09:30:00Z"]
		: ["2026-10-13T09:00:00Z", "2026-10-14T09:00:00Z", "2026-10-15T09:00:00Z"];

const previewQuery = (cron: string) => ({
	key: chatAutomationSchedulePreviewKey(organizationId, {
		schedule_cron: cron,
		schedule_time_zone: storyTimeZone,
	}),
	data: { next_run_times: nextRunTimes(cron) },
});

const modelCatalog: OrganizationChatModelsResponse = {
	models: [mockModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

const pinBrowserTimeZone = () => {
	const resolvedOptions = Intl.DateTimeFormat.prototype.resolvedOptions;
	spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockImplementation(
		function (this: Intl.DateTimeFormat) {
			return { ...resolvedOptions.call(this), timeZone: storyTimeZone };
		},
	);
};

const routeTo = (path: string, automationId?: string) =>
	reactRouterParameters({
		location: {
			path,
			pathParams: automationId ? { automationId } : {},
		},
		routing: reactRouterOutlet(
			{
				path: automationId
					? "/agents/automations/:automationId"
					: "/agents/automations/new",
			},
			<AgentAutomationEditorPage />,
		),
	});

// Gives the page the outlet context that the Agents layout provides.
const AgentsLayout: React.FC = () => (
	<div className="flex h-screen min-w-0 flex-col">
		<Outlet
			context={
				{
					chatErrorReasons: {},
					setChatErrorReason: () => {},
					clearChatErrorReason: () => {},
					navigateAfterArchive: () => {},
					onOpenRenameDialog: () => {},
					activeChatChildren: undefined,
					isSidebarCollapsed: false,
					onToggleSidebarCollapsed: () => {},
					onExpandSidebar: () => {},
					onChatReady: () => {},
				} satisfies AgentsPageOutletContext
			}
		/>
	</div>
);

const meta: Meta<typeof AgentsLayout> = {
	title: "pages/AgentsPage/AgentAutomationEditorPage",
	component: AgentsLayout,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		experiments: ["chat-automations"],
		organizations: [MockDefaultOrganization],
	},
	beforeEach: () => {
		pinBrowserTimeZone();
		localStorage.removeItem(selectedOrganizationIdStorageKey);
	},
};

export default meta;
type Story = StoryObj<typeof AgentsLayout>;

const commonQueries = [
	{ key: organizationChatModelsKey(organizationId), data: modelCatalog },
	{ key: chatEntityKey(mockTargetChat.id), data: mockTargetChat },
	previewQuery("0 9 * * *"),
	previewQuery("*/5 * * * *"),
];

export const NewSchedule: Story = {
	parameters: {
		reactRouter: routeTo("/agents/automations/new"),
		queries: commonQueries,
	},
};

export const NewWebhook: Story = {
	parameters: {
		reactRouter: routeTo("/agents/automations/new"),
		queries: commonQueries,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("radio", { name: "Webhook" }),
		);
	},
};

export const EditHeartbeat: Story = {
	parameters: {
		reactRouter: routeTo(`/agents/automations/${heartbeat.id}`, heartbeat.id),
		queries: [
			...commonQueries,
			{ key: chatAutomationsKey(organizationId), data: [heartbeat] },
		],
	},
};

// A viewer who may delete, but not update, another member's automation.
export const ReadOnlyWithDelete: Story = {
	parameters: {
		reactRouter: routeTo(
			`/agents/automations/${othersWebhook.id}`,
			othersWebhook.id,
		),
		queries: [
			...commonQueries,
			{ key: chatAutomationsKey(organizationId), data: [othersWebhook] },
			{
				key: getAuthorizationKey({
					checks: {
						canUpdateAny: {
							object: {
								resource_type: "chat_automation",
								organization_id: organizationId,
							},
							action: "update",
						},
						canDeleteAny: {
							object: {
								resource_type: "chat_automation",
								organization_id: organizationId,
							},
							action: "delete",
						},
					},
				}),
				data: { canUpdateAny: false, canDeleteAny: true },
			},
		],
	},
};
