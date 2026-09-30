import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, spyOn, userEvent, within } from "storybook/test";
import { API } from "#/api/api";
import { chatAutomationSchedulePreviewKey } from "#/api/queries/chatAutomations";
import { chat, organizationChatModelsKey } from "#/api/queries/chats";
import type {
	ChatAutomation,
	ChatModel,
	OrganizationChatModelsResponse,
} from "#/api/typesGenerated";
import { MockChat, MockChatAutomation } from "#/testHelpers/chatEntities";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockDefaultOrganization,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import { getPreferredTimezone } from "#/utils/timeZones";
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

const meta: Meta<typeof AutomationEditorDialog> = {
	title: "pages/AgentsPage/Automations/AutomationEditorDialog",
	component: AutomationEditorDialog,
	args: {
		organizationId,
		currentUserId: MockUserOwner.id,
		error: undefined,
		isSubmitting: false,
		onCreate: fn(),
		onUpdate: fn(),
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
					schedule_time_zone: getPreferredTimezone(),
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
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
					schedule_time_zone: getPreferredTimezone(),
				}),
				data: { next_run_times: nextRunTimes },
			},
		],
	},
	play: async () => {
		const body = within(document.body);
		await userEvent.click(
			await body.findByRole("radio", { name: "New chat each run" }),
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
			{ key: chat(MockChat.id).queryKey, data: MockChat },
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
		spyOn(API.experimental, "previewChatAutomationSchedule").mockRejectedValue(
			mockApiError({
				message: "Invalid chat automation.",
				validations: [
					{
						field: "schedule_cron",
						detail: "Expected exactly five fields.",
					},
				],
			}),
		);
	},
};

export const SaveForbidden: Story = {
	args: {
		automation: mockAutomation,
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
			{ key: chat(MockChat.id).queryKey, data: MockChat },
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
