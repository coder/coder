import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, spyOn, userEvent, waitFor } from "storybook/test";
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
import { MockDefaultOrganization, mockApiError } from "#/testHelpers/entities";
import { AutomationEditorForm } from "./AutomationEditorForm";

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

// Listed by the API but in another organization, so the field hides it.
const mockOtherOrgProject: ChatProject = {
	...MockChatProject,
	id: "9e8d7c6b-5a4f-4e3d-8c2b-1a0f9e8d7c6b",
	organization_id: "other-org-id",
	name: "Other organization project",
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

const meta: Meta<typeof AutomationEditorForm> = {
	title: "pages/AgentsPage/Automations/AutomationEditorForm",
	component: AutomationEditorForm,
	args: {
		organizationId,
		runsAsName: "Admin User",
		allowLeave: false,
		projectsEnabled: false,
		origin: "https://coder.example.com",
		error: undefined,
		isSubmitting: false,
		rotateSecretError: undefined,
		isRotatingSecret: false,
		onCreate: fn(),
		onUpdate: fn(),
		onRotateSecret: fn(),
		onCancel: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof AutomationEditorForm>;

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
				data: {
					next_run_times: nextRunTimes,
					clock_change_note:
						"When clocks go back, a time that occurs twice runs once, at its first occurrence. When clocks go forward, a time that is skipped runs at the first valid time after the change.",
				},
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

const mockEffortModel: ChatModel = {
	...mockModel,
	reasoning_efforts: ["low", "medium", "high"],
};

export const NewChatReasoningEffort: Story = {
	args: {
		automation: {
			...mockAutomation,
			target_mode: "new_chat",
			target_chat_id: undefined,
			new_chat_model_config_id: mockEffortModel.id,
			reasoning_effort: "high",
		},
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: { ...mockModelCatalog, models: [mockEffortModel] },
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
	play: async () => {
		await userEvent.click(
			await screen.findByRole("combobox", { name: "Reasoning effort" }),
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

export const EditWeekdaySchedule: Story = {
	args: { automation: { ...mockAutomation, schedule_cron: "30 9 * * 1-5" } },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "30 9 * * 1-5",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const EditCustomSchedule: Story = {
	args: { automation: { ...mockAutomation, schedule_cron: "0 9 1 * *" } },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "0 9 1 * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const EditHourlySchedule: Story = {
	args: { automation: { ...mockAutomation, schedule_cron: "15 * * * *" } },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: "15 * * * *",
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
};

export const SearchTimeZone: Story = {
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
			await screen.findByRole("button", { name: "Time zone" }),
		);
		await userEvent.type(
			await screen.findByPlaceholderText("Search time zones"),
			"berl",
		);
	},
};

export const EditBelowMinimumInterval: Story = {
	args: {
		automation: {
			...mockAutomation,
			schedule_cron: "*/2 * * * *",
			schedule_interval_below_minimum: true,
		},
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
					schedule_cron: "*/2 * * * *",
					schedule_time_zone: "UTC",
				}),
				data: {
					next_run_times: [
						"2026-10-01T09:00:00Z",
						"2026-10-01T09:02:00Z",
						"2026-10-01T09:04:00Z",
					],
					interval_warning:
						"Runs as often as every 2 minutes, but this deployment allows at most one run every 5 minutes. Saving a new or changed schedule like this fails.",
				},
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
			{ key: chatProjectsKey, data: [mockProject, mockOtherOrgProject] },
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
		await userEvent.click(
			await screen.findByRole("combobox", { name: /^Project/ }),
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
		error: mockApiError({
			message: "Forbidden.",
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

export const SaveValidationError: Story = {
	args: {
		automation: mockAutomation,
		error: mockApiError({
			message: "Validation failed.",
			validations: [
				{
					field: "reasoning_effort",
					detail: "Existing chat automations do not take a reasoning effort.",
				},
			],
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
	// Server errors show once the values they apply to were submitted.
	play: async () => {
		await userEvent.click(
			await screen.findByRole("button", { name: "Save changes" }),
		);
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

// The model popover fits the viewport and the effort select stays next to it.
export const NewChatModelPopover: Story = {
	args: { automation: mockNewChatAutomation },
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
	play: async () => {
		await userEvent.click(
			await screen.findByRole("combobox", { name: /^Model,/ }),
		);
	},
};

// The saved model was disabled, so the server pauses the automation.
export const EditModelUnavailable: Story = {
	args: {
		automation: {
			...mockNewChatAutomation,
			new_chat_model_config_id: "disabled-model",
			paused_reasons: ["model_unavailable"],
			next_run_times: [],
		},
	},
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatProjectsKey, data: [mockProject] },
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
		rotateSecretError: mockApiError({
			message: "Forbidden.",
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

// The cron line shows a whole five-field cron, and a long one stays inside
// the Trigger card. Pixel tests miss a clipped input, so the play checks it.
const expectCronFits = async (expected: string) => {
	const cron = await screen.findByRole("textbox", {
		name: "Cron expression (required)",
	});
	await waitFor(() => expect(cron).toHaveValue(expected));
	const card = cron.closest("section")?.querySelector(":scope > .rounded-lg");
	if (!(card instanceof HTMLElement)) {
		throw new Error("The cron input is not inside the Trigger card.");
	}
	const input = cron.getBoundingClientRect();
	const cardBox = card.getBoundingClientRect();
	expect(input.right).toBeLessThanOrEqual(cardBox.right);
	expect(input.left).toBeGreaterThanOrEqual(cardBox.left);
	return cron as HTMLInputElement;
};

export const CronFitsItsValue: Story = {
	args: { automation: mockAutomation },
	parameters: Edit.parameters,
	play: async () => {
		const cron = await expectCronFits("0 9 * * *");
		// Nothing of a short cron is cut off.
		expect(cron.scrollWidth).toBeLessThanOrEqual(cron.clientWidth);
	},
};

const longCron = `${Array.from({ length: 60 }, (_, minute) => minute).join(",")} 0,2,4,6,8,10,12,14,16,18,20,22 1,3,5,7,9,11,13,15,17,19,21,23,25,27,29,31 * 1-5`;

export const LongCronStaysInsideTheCard: Story = {
	args: { automation: { ...mockAutomation, schedule_cron: longCron } },
	parameters: {
		queries: [
			{
				key: organizationChatModelsKey(organizationId),
				data: mockModelCatalog,
			},
			{ key: chatEntityKey(MockChat.id), data: MockChat },
			{
				key: chatAutomationSchedulePreviewKey(organizationId, {
					schedule_cron: longCron,
					schedule_time_zone: "UTC",
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	play: async () => {
		expect(longCron.length).toBeGreaterThan(200);
		await expectCronFits(longCron);
	},
};
