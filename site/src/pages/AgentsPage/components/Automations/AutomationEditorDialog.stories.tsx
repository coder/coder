import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { chatAutomationSchedulePreviewKey } from "#/api/queries/chatAutomations";
import { chatProjectsKey } from "#/api/queries/chatProjects";
import { chatEntityKey, organizationChatModelsKey } from "#/api/queries/chats";
import type {
	ChatAutomation,
	ChatModel,
	ChatProject,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import {
	MockChat,
	MockChatAutomation,
	MockChatProject,
	MockWebhookChatAutomation,
} from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { AutomationEditorDialog } from "./AutomationEditorDialog";

const organizationId = MockDefaultOrganization.id;

const mockModel: ChatModel = {
	...MockChatModel,
	organization_id: organizationId,
};

const mockAutomation: ChatAutomation = {
	...MockChatAutomation,
	organization_id: organizationId,
	target_chat_id: MockChat.id,
};

const mockWebhookAutomation: ChatAutomation = {
	...MockWebhookChatAutomation,
	organization_id: organizationId,
	target_chat_id: MockChat.id,
};

const mockProject: ChatProject = {
	...MockChatProject,
	organization_id: organizationId,
};

const mockNewChatAutomation: ChatAutomation = {
	...mockAutomation,
	target_mode: "new_chat",
	target_chat_id: undefined,
	when_busy: undefined,
	new_chat_model_config_id: mockModel.id,
	project_id: mockProject.id,
};

const nextRunTimes = [
	"2026-10-01T09:00:00Z",
	"2026-10-02T09:00:00Z",
	"2026-10-03T09:00:00Z",
];

const mockModelCatalog: OrganizationChatModelsResponse = {
	models: [mockModel],
	providers: [MockChatModelProviderDescriptor],
	unsupported_providers: [],
};

// New automations default to the browser's zone. Pin it so the time zone
// select and the upcoming runs render the same on every host.
const storyTimeZone = "America/New_York";

const pinBrowserTimeZone = () => {
	const resolvedOptions = Intl.DateTimeFormat.prototype.resolvedOptions;
	spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockImplementation(
		function (this: Intl.DateTimeFormat) {
			return { ...resolvedOptions.call(this), timeZone: storyTimeZone };
		},
	);
};

const rejectChat = (status: number) => () => {
	spyOn(API.experimental, "getChat").mockRejectedValue({
		...mockApiError({ message: "Chat error." }),
		status,
	});
};

const meta: Meta<typeof AutomationEditorDialog> = {
	title: "pages/AgentsPage/Automations/AutomationEditorDialog",
	component: AutomationEditorDialog,
	args: {
		organizationId,
		currentUserId: MockUserOwner.id,
		projectsEnabled: false,
		origin: "https://coder.example.com",
		error: undefined,
		isSubmitting: false,
		rotateSecretError: undefined,
		isRotatingSecret: false,
		onCreate: fn(),
		onUpdate: fn(),
		onRotateSecret: fn(),
		onClose: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AutomationEditorDialog>;

export const CreateSchedule: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: storyTimeZone,
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: pinBrowserTimeZone,
};

export const NewChatTarget: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: storyTimeZone,
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: pinBrowserTimeZone,
	play: async () => {
		await userEvent.click(
			await screen.findByRole("radio", { name: "New chat each run" }),
		);
	},
};

export const Edit: Story = {
	args: { automation: mockAutomation },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const CreateNewChatProject: Story = {
	args: { projectsEnabled: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatProjectsKey, data: [mockProject] },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: storyTimeZone,
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: pinBrowserTimeZone,
	play: async () => {
		await userEvent.click(
			await screen.findByRole("radio", { name: "New chat each run" }),
		);
	},
};

export const EditNewChatProject: Story = {
	args: { automation: mockNewChatAutomation, projectsEnabled: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatProjectsKey, data: [mockProject] },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const EditNewChatUnknownProject: Story = {
	args: {
		automation: { ...mockNewChatAutomation, project_id: "project-gone" },
		projectsEnabled: true,
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatProjectsKey, data: [mockProject] },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const ProjectsLoading: Story = {
	args: { automation: mockNewChatAutomation, projectsEnabled: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockReturnValue(
			new Promise(() => {}),
		);
	},
};

export const NoProjects: Story = {
	args: {
		automation: { ...mockNewChatAutomation, project_id: undefined },
		projectsEnabled: true,
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatProjectsKey, data: [] },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const ProjectsLoadError: Story = {
	args: { automation: mockNewChatAutomation, projectsEnabled: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjects").mockRejectedValue(
			mockApiError({ message: "Projects error." }),
		);
	},
};

export const TargetChatNotFound: Story = {
	args: { automation: mockAutomation },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: rejectChat(404),
};

export const TargetChatLoadError: Story = {
	args: { automation: mockAutomation },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	beforeEach: rejectChat(500),
};

// The server's tzdata can know zones that this browser rejects, so the
// upcoming runs fall back to UTC.
export const UnknownTimeZonePreview: Story = {
	args: {
		automation: { ...mockAutomation, schedule_time_zone: "Mars/Olympus" },
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "Mars/Olympus",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const PreviewError: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
		],
	},
	beforeEach: () => {
		spyOn(API.experimental, "previewChatAutomationSchedule").mockRejectedValue({
			...mockApiError({
				message: "Invalid chat automation.",
				validations: [
					{
						field: "schedule_cron",
						detail: "Expected exactly five fields.",
					},
				],
			}),
			status: 400,
		});
	},
};

export const SaveForbidden: Story = {
	args: {
		automation: mockAutomation,
		currentUserId: "another-user",
		error: mockApiError({
			message: "Only the owner of a chat automation can change it.",
		}),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const CreateWebhook: Story = {
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 * * *",
					schedule_time_zone: storyTimeZone,
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("radio", { name: "Webhook" }),
		);
	},
};

export const EditWebhook: Story = {
	args: { automation: mockWebhookAutomation },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
};

export const RotatingSecret: Story = {
	args: { automation: mockWebhookAutomation, isRotatingSecret: true },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
};

export const EditUsedSingleUseWebhook: Story = {
	args: {
		automation: {
			...mockWebhookAutomation,
			webhook_use: "single",
			webhook_consumed_at: "2026-09-30T10:15:00Z",
		},
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
	// formatDate renders in the browser's zone, which resolvedOptions does not
	// control. Pin it so "Used on" renders the same on every host.
	beforeEach: () => {
		const toLocaleDateString = Date.prototype.toLocaleDateString;
		spyOn(Date.prototype, "toLocaleDateString").mockImplementation(function (
			this: Date,
			locales?: Intl.LocalesArgument,
			options?: Intl.DateTimeFormatOptions,
		) {
			return toLocaleDateString.call(this, locales, {
				...options,
				timeZone: options?.timeZone ?? storyTimeZone,
			});
		});
	},
};

export const ConfirmRotateSecret: Story = {
	args: { automation: mockWebhookAutomation },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Rotate secret" }),
		);
	},
};

export const RotateSecretForbidden: Story = {
	args: {
		automation: mockWebhookAutomation,
		currentUserId: "another-user",
		rotateSecretError: mockApiError({
			message: "Only the owner of a chat automation can change it.",
		}),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
};

export const RotateSecretConflict: Story = {
	args: {
		// The editor still shows the webhook as unused while the server has
		// already consumed it.
		automation: { ...mockWebhookAutomation, webhook_use: "single" },
		rotateSecretError: mockApiError({
			message: "This single-use webhook was already used.",
			detail: "Its secret can no longer be rotated.",
		}),
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
		],
	},
};
