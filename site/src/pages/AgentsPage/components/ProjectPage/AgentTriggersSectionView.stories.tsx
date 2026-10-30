import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn, screen, spyOn, userEvent } from "storybook/test";
import { API } from "#/api/api";
import { chatProjectTriggerDeliveriesKey } from "#/api/queries/chatProjects";
import type {
	ChatProjectTrigger,
	ChatProjectTriggerDelivery,
} from "#/api/typesGenerated";
import {
	MockChatProject,
	MockChatProjectScheduleTrigger,
	MockChatProjectSlackTrigger,
	MockChatProjectTrigger,
	MockChatProjectTriggerDeliveries,
	MockChatProjectTriggerDisabled,
	mockApiError,
} from "#/testHelpers/entities";
import { AgentTriggersSectionView } from "./AgentTriggersSection";

const mockFilteredGitHubTrigger: ChatProjectTrigger = {
	...MockChatProjectTrigger,
	id: "chat-project-trigger-github-filtered",
	name: "Review comments",
	events: ["issue_comment.created", "pull_request_review.submitted"],
	senders: ["octocat", "hubot"],
	repository: undefined,
	text_pattern: "^/review",
	text_pattern_is_regex: true,
	prompt: "",
};

const mockNewSlackTrigger: ChatProjectTrigger = {
	...MockChatProjectSlackTrigger,
	id: "chat-project-trigger-slack-new",
	name: "Any message in C0123456789",
	events: ["message"],
	text_pattern: undefined,
	has_slack_signing_secret: false,
	last_triggered_at: undefined,
	last_delivery: undefined,
};

const mockNewSlackMentionTrigger: ChatProjectTrigger = {
	...mockNewSlackTrigger,
	id: "chat-project-trigger-slack-mention-new",
	name: "App mention",
	events: ["app_mention"],
	channel: undefined,
};

const mockContinuingGitHubTrigger: ChatProjectTrigger = {
	...MockChatProjectTrigger,
	id: "chat-project-trigger-github-continuing",
	name: "Answer review comments",
	events: ["issue_comment.created", "pull_request_review.submitted"],
	continue_conversations: true,
	busy_behavior: "interrupt",
};

const mockContinuingWebhookTrigger: ChatProjectTrigger = {
	...MockChatProjectTriggerDisabled,
	id: "chat-project-trigger-webhook-continuing",
	name: "Support tickets",
	enabled: true,
	continue_conversations: true,
};

const mockNewGitHubTrigger: ChatProjectTrigger = {
	...MockChatProjectTrigger,
	id: "chat-project-trigger-github-new",
	name: "Issue opened",
	events: ["issues.opened"],
	repository: undefined,
	last_triggered_at: undefined,
	last_delivery: undefined,
};

const mockWebhookURL =
	"https://coder.example.com/api/experimental/chat-triggers/3f1c/webhook/abc123";

const mockCustomScheduleTrigger: ChatProjectTrigger = {
	...MockChatProjectScheduleTrigger,
	id: "chat-project-trigger-schedule-custom",
	name: "Monthly dependency report",
	schedule: "CRON_TZ=Europe/Berlin 30 8 1 * *",
	continue_conversations: true,
};

/** Its runs continue one chat, so a run can show as continued. */
const mockContinuingScheduleTrigger: ChatProjectTrigger = {
	...MockChatProjectScheduleTrigger,
	continue_conversations: true,
};

const mockScheduleStartedDelivery: ChatProjectTriggerDelivery = {
	id: "chat-project-trigger-delivery-schedule-started",
	trigger_id: MockChatProjectScheduleTrigger.id,
	created_at: "2026-09-04T09:00:00Z",
	status: "started",
	event: "schedule",
	chat_id: "run-1",
};

const mockScheduleRunNowDelivery: ChatProjectTriggerDelivery = {
	id: "chat-project-trigger-delivery-schedule-run-now",
	trigger_id: MockChatProjectScheduleTrigger.id,
	created_at: "2026-09-04T10:15:00Z",
	status: "continued",
	event: "run_now",
	chat_id: "run-1",
};

const mockScheduleSkippedDelivery: ChatProjectTriggerDelivery = {
	id: "chat-project-trigger-delivery-schedule-skipped",
	trigger_id: MockChatProjectScheduleTrigger.id,
	created_at: "2026-09-03T09:00:00Z",
	status: "rejected",
	event: "schedule",
	message:
		"The trigger's chat has too many queued messages. The run was skipped.",
};

const mockDisabledScheduleTrigger: ChatProjectTrigger = {
	...MockChatProjectScheduleTrigger,
	id: "chat-project-trigger-schedule-off",
	name: "Hourly status check",
	schedule: "CRON_TZ=UTC 15 * * * *",
	enabled: false,
	next_run_at: undefined,
	last_triggered_at: undefined,
	last_delivery: undefined,
};

const meta: Meta<typeof AgentTriggersSectionView> = {
	title: "pages/AgentsPage/ProjectPage/AgentTriggersSectionView",
	component: AgentTriggersSectionView,
	decorators: [
		(Story) => (
			<div className="max-w-3xl">
				<Story />
			</div>
		),
	],
	args: {
		triggers: [
			MockChatProjectTrigger,
			MockChatProjectSlackTrigger,
			mockFilteredGitHubTrigger,
			MockChatProjectTriggerDisabled,
			MockChatProjectScheduleTrigger,
		],
		project: MockChatProject,
		canManage: true,
		error: undefined,
		onRetry: fn(),
		isCreating: false,
		revealed: undefined,
		onDismissRevealed: fn(),
		onCreate: fn(),
		onAddSchedule: fn(),
		onUpdate: fn(),
		onRegenerateURL: fn(),
		onRunNow: fn(),
		onDelete: fn(),
		// Pinned so relative and exact times stay the same in screenshots.
		now: new Date("2026-09-04T12:00:00Z"),
		timeZone: "UTC",
	},
};

export default meta;
type Story = StoryObj<typeof AgentTriggersSectionView>;

export const Default: Story = {};

export const ReadOnly: Story = { args: { canManage: false } };

export const ReadOnlyLongValues: Story = {
	args: {
		canManage: false,
		triggers: [
			{
				...mockFilteredGitHubTrigger,
				repository: "coder/a-repository-with-a-very-long-name-for-testing",
				text_pattern:
					"^/(review|re-review|approve|request-changes)\\s+(now|later)$",
				prompt:
					"Review the pull request, leave inline comments on anything risky, and summarize the findings in one comment.",
			},
		],
	},
};

export const Empty: Story = { args: { triggers: [] } };

export const Loading: Story = { args: { triggers: undefined } };

export const LoadError: Story = {
	args: {
		triggers: undefined,
		error: mockApiError({ message: "Failed to load triggers." }),
	},
};

export const RefetchError: Story = {
	args: { error: mockApiError({ message: "Failed to load triggers." }) },
};

export const Creating: Story = { args: { isCreating: true } };

export const NotSetUpWarnings: Story = {
	args: { triggers: [mockNewSlackTrigger, mockNewGitHubTrigger] },
};

export const WebhookURLShownOnce: Story = {
	args: {
		triggers: [mockNewGitHubTrigger],
		revealed: { trigger: mockNewGitHubTrigger, webhook_url: mockWebhookURL },
	},
};

export const WebhookURLSetupSteps: Story = {
	args: WebhookURLShownOnce.args,
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Set it up in GitHub" }),
		);
	},
};

export const SlackSetupSteps: Story = {
	args: {
		triggers: [mockNewSlackTrigger],
		revealed: { trigger: mockNewSlackTrigger, webhook_url: mockWebhookURL },
	},
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Set it up in Slack" }),
		);
	},
};

/** Thread replies need the message bot events even for a mention trigger. */
export const SlackMentionSetupSteps: Story = {
	args: {
		triggers: [mockNewSlackMentionTrigger],
		revealed: {
			trigger: mockNewSlackMentionTrigger,
			webhook_url: mockWebhookURL,
		},
	},
	play: SlackSetupSteps.play,
};

export const SlackSetupStepsSigningSecretSet: Story = {
	args: {
		triggers: [MockChatProjectSlackTrigger],
		revealed: {
			trigger: MockChatProjectSlackTrigger,
			webhook_url: mockWebhookURL,
		},
	},
	play: SlackSetupSteps.play,
};

export const CustomWebhookSetupSteps: Story = {
	args: {
		triggers: [MockChatProjectTriggerDisabled],
		revealed: {
			trigger: MockChatProjectTriggerDisabled,
			webhook_url: mockWebhookURL,
		},
	},
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "How to call it" }),
		);
	},
};

export const CustomWebhookConversationSetupSteps: Story = {
	args: {
		triggers: [mockContinuingWebhookTrigger],
		revealed: {
			trigger: mockContinuingWebhookTrigger,
			webhook_url: mockWebhookURL,
		},
	},
	play: CustomWebhookSetupSteps.play,
};

export const ConversationOptions: Story = {
	args: {
		triggers: [
			mockContinuingGitHubTrigger,
			MockChatProjectSlackTrigger,
			mockNewSlackTrigger,
			mockContinuingWebhookTrigger,
			MockChatProjectTrigger,
		],
	},
};

export const ConversationOptionsReadOnly: Story = {
	args: { ...ConversationOptions.args, canManage: false },
};

export const ConversationEditor: Story = {
	args: { triggers: [MockChatProjectSlackTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: "Replies in the thread: continue the same chat",
			}),
		);
	},
};

export const BusyEditor: Story = {
	args: { triggers: [mockContinuingGitHubTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `If ${MockChatProject.name} is busy: interrupt it`,
			}),
		);
	},
};

export const SigningSecretSet: Story = {
	args: { triggers: [MockChatProjectSlackTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Slack signing secret: Set" }),
		);
	},
};

export const SigningSecretNotSet: Story = {
	args: { triggers: [mockNewSlackTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Slack signing secret: Not set" }),
		);
	},
};

const mockVerifiedGitHubTrigger: ChatProjectTrigger = {
	...MockChatProjectTrigger,
	id: "chat-project-trigger-github-verified",
	name: "Verified pull requests",
	has_github_webhook_secret: true,
};

export const GitHubWebhookSecretNotSet: Story = {
	args: { triggers: [MockChatProjectTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "GitHub webhook secret: Not set" }),
		);
	},
};

export const GitHubWebhookSecretSet: Story = {
	args: { triggers: [mockVerifiedGitHubTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "GitHub webhook secret: Set" }),
		);
	},
};

/** Filters without a secret warn; verified and unfiltered triggers do not. */
export const UnverifiedFilterWarnings: Story = {
	args: {
		triggers: [
			MockChatProjectTrigger,
			mockNewSlackTrigger,
			mockVerifiedGitHubTrigger,
			mockNewGitHubTrigger,
		],
	},
};

export const AddMenu: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Add trigger" }));
	},
};

export const AddMenuGitHubEvents: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Add trigger" }));
		await userEvent.click(
			await screen.findByRole("option", { name: "GitHub" }),
		);
	},
};

export const AddMenuSearch: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "Add trigger" }));
		await userEvent.type(
			await screen.findByPlaceholderText("Search triggers..."),
			"mention",
		);
	},
};

export const SlackMessageEditor: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: /^Messages: / }));
	},
};

export const GitHubEventsEditor: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Events: Pull request opened" }),
		);
	},
};

export const SendersEditor: Story = {
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: "By: 2 people" }));
	},
};

export const RepositoryEditorInvalid: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", { name: "Repository: coder/coder" }),
		);
		const input = await screen.findByLabelText("Repository");
		await userEvent.clear(input);
		await userEvent.type(input, "coder");
	},
};

export const PromptEditor: Story = {
	play: async () => {
		await userEvent.click(
			screen.getAllByRole("button", {
				name: new RegExp(`^Ask ${MockChatProject.name} to: `),
			})[0],
		);
	},
};

export const MoreMenu: Story = {
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `More actions for ${MockChatProjectTrigger.name}`,
			}),
		);
	},
};

export const DeliveryStatuses: Story = {
	args: {
		triggers: MockChatProjectTriggerDeliveries.map((delivery, index) => ({
			...MockChatProjectTrigger,
			id: `chat-project-trigger-status-${index}`,
			name: `Trigger ${index + 1}`,
			last_delivery: delivery,
		})),
	},
};

export const RecentDeliveries: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectTriggerDeliveriesKey(
					MockChatProject.id,
					MockChatProjectTrigger.id,
				),
				data: MockChatProjectTriggerDeliveries,
			},
		],
	},
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `Recent deliveries for ${MockChatProjectTrigger.name}`,
			}),
		);
	},
};

export const RecentDeliveriesEmpty: Story = {
	parameters: {
		queries: [
			{
				key: chatProjectTriggerDeliveriesKey(
					MockChatProject.id,
					MockChatProjectTrigger.id,
				),
				data: [],
			},
		],
	},
	play: RecentDeliveries.play,
};

export const RecentDeliveriesLoading: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatProjectTriggerDeliveries").mockReturnValue(
			new Promise(() => undefined),
		);
	},
	play: RecentDeliveries.play,
};

export const RecentDeliveriesError: Story = {
	beforeEach: () => {
		spyOn(
			API.experimental,
			"getChatProjectTriggerDeliveries",
		).mockRejectedValue(
			mockApiError({ message: "Failed to load deliveries." }),
		);
	},
	play: RecentDeliveries.play,
};

export const ScheduleTriggers: Story = {
	args: {
		triggers: [
			MockChatProjectScheduleTrigger,
			mockCustomScheduleTrigger,
			mockDisabledScheduleTrigger,
		],
	},
};

export const ScheduleTriggersReadOnly: Story = {
	args: { ...ScheduleTriggers.args, canManage: false },
};

export const ScheduleEditor: Story = {
	args: { triggers: [MockChatProjectScheduleTrigger] },
	play: async () => {
		await userEvent.click(screen.getByRole("button", { name: /^Schedule: / }));
	},
};

export const ScheduleEditorCustom: Story = {
	args: { triggers: [mockCustomScheduleTrigger] },
	play: ScheduleEditor.play,
};

export const ScheduleEditorTooFrequent: Story = {
	args: ScheduleEditorCustom.args,
	play: async (context) => {
		await ScheduleEditor.play?.(context);
		const cron = screen.getByLabelText("Cron expression");
		await userEvent.clear(cron);
		await userEvent.type(cron, "*/2 * * * *");
	},
};

export const ScheduleEditorHourly: Story = {
	args: { triggers: [{ ...mockDisabledScheduleTrigger, enabled: true }] },
	play: ScheduleEditor.play,
};

export const ScheduleMoreMenu: Story = {
	args: { triggers: [MockChatProjectScheduleTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `More actions for ${MockChatProjectScheduleTrigger.name}`,
			}),
		);
	},
};

export const ScheduleMoreMenuRunning: Story = {
	args: {
		...ScheduleMoreMenu.args,
		runningTriggerId: MockChatProjectScheduleTrigger.id,
	},
	play: ScheduleMoreMenu.play,
};

export const ScheduleMoreMenuOff: Story = {
	args: { triggers: [mockDisabledScheduleTrigger] },
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `More actions for ${mockDisabledScheduleTrigger.name}`,
			}),
		);
	},
};

export const ScheduleRecentRuns: Story = {
	args: { triggers: [mockContinuingScheduleTrigger] },
	parameters: {
		queries: [
			{
				key: chatProjectTriggerDeliveriesKey(
					MockChatProject.id,
					mockContinuingScheduleTrigger.id,
				),
				data: [
					mockScheduleRunNowDelivery,
					mockScheduleStartedDelivery,
					mockScheduleSkippedDelivery,
				],
			},
		],
	},
	play: async () => {
		await userEvent.click(
			screen.getByRole("button", {
				name: `Recent runs for ${mockContinuingScheduleTrigger.name}`,
			}),
		);
	},
};
