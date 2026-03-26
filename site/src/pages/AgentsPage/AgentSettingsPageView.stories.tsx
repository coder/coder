import { MockTemplate, MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import dayjs from "dayjs";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import { userKey } from "#/api/queries/users";
import type * as TypesGen from "#/api/typesGenerated";
import { AgentSettingsPageView } from "./AgentSettingsPageView";

// ── Usage mock helpers ─────────────────────────────────────────

const mockUsers: TypesGen.ChatCostUserRollup[] = [
	{
		user_id: "user-1",
		username: "alice",
		name: "Alice Liddell",
		avatar_url: "",
		total_cost_micros: 2_500_000,
		message_count: 42,
		chat_count: 5,
		total_input_tokens: 200_000,
		total_output_tokens: 300_000,
		total_cache_read_tokens: 10_000,
		total_cache_creation_tokens: 5_000,
	},
	{
		user_id: "user-2",
		username: "bob",
		name: "Bob Builder",
		avatar_url: "",
		total_cost_micros: 1_000_000,
		message_count: 18,
		chat_count: 3,
		total_input_tokens: 80_000,
		total_output_tokens: 120_000,
		total_cache_read_tokens: 4_000,
		total_cache_creation_tokens: 2_000,
	},
];

const mockUsersResponse: TypesGen.ChatCostUsersResponse = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	count: mockUsers.length,
	users: mockUsers,
};

const mockUserProfile: TypesGen.User = {
	id: "user-1",
	username: "alice",
	name: "Alice Liddell",
	email: "alice@example.com",
	avatar_url: "",
	created_at: "2025-01-01T00:00:00Z",
	updated_at: "2025-06-01T00:00:00Z",
	status: "active",
	organization_ids: [],
	roles: [],
	last_seen_at: "2026-03-11T10:00:00Z",
	login_type: "password",
};

const mockCostSummary: TypesGen.ChatCostSummary = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 2_500_000,
	priced_message_count: 40,
	unpriced_message_count: 2,
	total_input_tokens: 200_000,
	total_output_tokens: 300_000,
	total_cache_read_tokens: 10_000,
	total_cache_creation_tokens: 5_000,
	by_model: [
		{
			model_config_id: "model-1",
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 2_000_000,
			message_count: 30,
			total_input_tokens: 150_000,
			total_output_tokens: 250_000,
			total_cache_read_tokens: 8_000,
			total_cache_creation_tokens: 4_000,
		},
	],
	by_chat: [
		{
			root_chat_id: "chat-1",
			chat_title: "Refactor auth module",
			total_cost_micros: 1_200_000,
			message_count: 15,
			total_input_tokens: 80_000,
			total_output_tokens: 120_000,
			total_cache_read_tokens: 3_000,
			total_cache_creation_tokens: 1_500,
		},
	],
};

/**
 * Set up spies for all usage-related API methods. The behaviour mocks
 * (system prompt, desktop, custom prompt) are still inherited from
 * the meta-level `beforeEach`.
 */
const setupUsageSpies = (opts?: {
	usersResponse?: TypesGen.ChatCostUsersResponse;
}) => {
	spyOn(API.experimental, "getChatCostUsers").mockResolvedValue(
		opts?.usersResponse ?? mockUsersResponse,
	);
	spyOn(API, "getUser").mockResolvedValue(mockUserProfile);
	spyOn(API.experimental, "getChatCostSummary").mockResolvedValue(
		mockCostSummary,
	);
};

const getChatCostUsersCalls = () =>
	(
		API.experimental
			.getChatCostUsers as typeof API.experimental.getChatCostUsers & {
			mock: {
				calls: Array<[Parameters<typeof API.experimental.getChatCostUsers>[0]]>;
			};
		}
	).mock.calls;

const fixedNow = dayjs("2026-03-12T00:00:00Z");

// ── Meta ───────────────────────────────────────────────────────

const meta = {
	title: "pages/AgentsPage/AgentSettingsPageView",
	component: AgentSettingsPageView,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		activeSection: "behavior",
		canManageChatModelConfigs: false,
		canSetSystemPrompt: true,
		now: fixedNow,
	},
	parameters: {
		user: MockUserOwner,
		layout: "fullscreen",
	},
	beforeEach: () => {
		spyOn(API.experimental, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
		});
		spyOn(API.experimental, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API.experimental, "getChatDesktopEnabled").mockResolvedValue({
			enable_desktop: false,
		});
		spyOn(API.experimental, "updateChatDesktopEnabled").mockResolvedValue();
		spyOn(API.experimental, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API.experimental, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API.experimental, "getChatModelConfigs").mockResolvedValue([]);
		spyOn(
			API.experimental,
			"getUserChatCompactionThresholds",
		).mockResolvedValue({
			thresholds: [],
		});
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 0,
		});
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockResolvedValue();
		spyOn(API.experimental, "getChatTemplateAllowlist").mockResolvedValue({
			template_ids: [],
		});
		spyOn(API.experimental, "updateChatTemplateAllowlist").mockResolvedValue();
		spyOn(API, "getTemplates").mockResolvedValue([
			{
				...MockTemplate,
				id: "abc-123",
				name: "docker-dev",
				display_name: "Docker Development",
			},
			{
				...MockTemplate,
				id: "def-456",
				name: "kubernetes-prod",
				display_name: "Kubernetes Production",
			},
			{
				...MockTemplate,
				id: "ghi-789",
				name: "aws-windows",
				display_name: "AWS Windows Desktop",
			},
		]);
	},
} satisfies Meta<typeof AgentSettingsPageView>;

export default meta;
type Story = StoryObj<typeof AgentSettingsPageView>;

// ── Behavior tab stories ───────────────────────────────────────

export const DesktopSetting: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Virtual Desktop");
		await canvas.findByText(
			/Allow agents to use a virtual, graphical desktop/i,
		);
		await canvas.findByRole("switch", { name: "Enable" });
	},
};

export const TogglesDesktop: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const toggle = await canvas.findByRole("switch", {
			name: "Enable",
		});

		await userEvent.click(toggle);
		await waitFor(() => {
			expect(API.experimental.updateChatDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};

export const DefaultAutostopDefault: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Workspace Autostop Fallback");
		// Description is always visible.
		await canvas.findByText(
			/set a default autostop for agent-created workspaces/i,
		);

		// Toggle should be OFF when TTL is 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).not.toBeChecked();

		// Duration field should not be visible when disabled.
		expect(canvas.queryByLabelText("Autostop Fallback")).toBeNull();
	},
};

export const DefaultAutostopCustomValue: Story = {
	beforeEach: () => {
		// 2h = 2 hours exactly, shows cleanly in DurationField.
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 7_200_000,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle should be ON when TTL > 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		// Duration field should be visible with 2 hours.
		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");
	},
};

export const DefaultAutostopSave: Story = {
	beforeEach: () => {
		let currentTTL = 0;
		spyOn(API.experimental, "getChatWorkspaceTTL").mockImplementation(
			async () => ({ workspace_ttl_ms: currentTTL }),
		);
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockImplementation(
			async (req) => {
				currentTTL = req.workspace_ttl_ms;
			},
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle ON — should auto-save with 1-hour default.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		await userEvent.click(toggle);

		await waitFor(() => {
			expect(API.experimental.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 3_600_000,
			});
		});

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("1");

		// Change to 3 hours — Save button should appear.
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "3");

		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(API.experimental.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 10_800_000,
			});
		});

		// Verify the isTTLZero guard: clearing to 0 should disable Save
		// because the toggle is still ON.
		await userEvent.clear(durationInput);
		await waitFor(() => {
			expect(saveButton).toBeDisabled();
		});
	},
};

export const DefaultAutostopExceedsMax: Story = {
	beforeEach: () => {
		let currentTTL = 0;
		spyOn(API.experimental, "getChatWorkspaceTTL").mockImplementation(
			async () => ({ workspace_ttl_ms: currentTTL }),
		);
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockImplementation(
			async (req) => {
				currentTTL = req.workspace_ttl_ms;
			},
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle ON to reveal the duration field.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		await userEvent.click(toggle);

		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		const ttlForm = durationInput.closest("form")!;

		// Enter 721 hours (exceeds 30-day / 720h limit).
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "721");

		// Error helper text should appear.
		await waitFor(() => {
			expect(canvas.getByText(/must not exceed 30 days/i)).toBeInTheDocument();
		});

		// Save button should be disabled despite the field being dirty.
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
	},
};

export const DefaultAutostopToggleOff: Story = {
	beforeEach: () => {
		let currentTTL = 7_200_000;
		spyOn(API.experimental, "getChatWorkspaceTTL").mockImplementation(
			async () => ({ workspace_ttl_ms: currentTTL }),
		);
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockImplementation(
			async (req) => {
				currentTTL = req.workspace_ttl_ms;
			},
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle should start ON since TTL > 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		// Click toggle OFF.
		await userEvent.click(toggle);

		await waitFor(() => {
			expect(API.experimental.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 0,
			});
		});

		// Duration field should no longer be visible.
		await waitFor(() => {
			expect(canvas.queryByLabelText("Autostop Fallback")).toBeNull();
		});
	},
};

export const DefaultAutostopSaveDisabled: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 7_200_000,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle should be ON since TTL > 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		// Duration field should show 2 hours.
		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");

		// Save button should exist but be disabled (no changes made).
		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
	},
};

export const DefaultAutostopToggleFailure: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 0,
		});
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockRejectedValue(
			new Error("Server error"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle starts OFF since TTL is 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).not.toBeChecked();

		// Click toggle ON.
		await userEvent.click(toggle);

		// Verify the mutation was called with the 1-hour default.
		await waitFor(() => {
			expect(API.experimental.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 3_600_000,
			});
		});

		// The onError handler resets state, reverting the toggle.
		await waitFor(() => {
			expect(toggle).not.toBeChecked();
		});

		// Error message should be visible.
		expect(
			canvas.getByText("Failed to save autostop setting."),
		).toBeInTheDocument();

		// DurationField should not be visible since toggle reverted to OFF.
		expect(canvas.queryByLabelText("Autostop Fallback")).toBeNull();
	},
};

export const DefaultAutostopToggleOffFailure: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 7_200_000,
		});
		spyOn(API.experimental, "updateChatWorkspaceTTL").mockRejectedValue(
			new Error("Server error"),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Toggle starts ON since TTL > 0.
		const toggle = await canvas.findByRole("switch", {
			name: "Enable default autostop",
		});
		expect(toggle).toBeChecked();

		// Duration should show 2 hours initially.
		const durationInput = await canvas.findByLabelText("Autostop Fallback");
		expect(durationInput).toHaveValue("2");

		// Click toggle OFF.
		await userEvent.click(toggle);

		// Verify the mutation was called with 0 to disable.
		await waitFor(() => {
			expect(API.experimental.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 0,
			});
		});

		// The onError handler resets state, reverting the toggle to ON.
		await waitFor(() => {
			expect(toggle).toBeChecked();
		});

		// Error message should be visible.
		expect(
			canvas.getByText("Failed to save autostop setting."),
		).toBeInTheDocument();

		// DurationField should still be visible with 2 hours.
		expect(canvas.getByLabelText("Autostop Fallback")).toHaveValue("2");
	},
};

export const DefaultAutostopNotVisibleToNonAdmin: Story = {
	args: {
		canSetSystemPrompt: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Personal Instructions should be visible.
		await canvas.findByText("Personal Instructions");

		// Admin-only sections should not be present.
		const ttlHeading = canvas.queryByText("Workspace Autostop Fallback");
		expect(ttlHeading).toBeNull();

		const desktopHeading = canvas.queryByText("Virtual Desktop");
		expect(desktopHeading).toBeNull();
	},
};

// ── Usage tab stories ──────────────────────────────────────────

export const UsageUserList: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// The section header should be visible.
		await canvas.findByText("Usage");

		// Both users should appear in the table.
		await expect(await canvas.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(canvas.getByText("Bob Builder")).toBeInTheDocument();

		// Verify the search field is present.
		await expect(
			canvas.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};

export const UsageDateFilter: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const defaultStartDate = fixedNow.subtract(30, "day").toISOString();
		const defaultStartLabel = fixedNow
			.subtract(30, "day")
			.format("MMM D, YYYY");
		const defaultEndLabel = fixedNow.format("MMM D, YYYY");

		await waitFor(() => {
			expect(API.experimental.getChatCostUsers).toHaveBeenCalled();
		});
		const initialCallCount = getChatCostUsersCalls().length;

		const dateRangeTrigger = await canvas.findByRole("button", {
			name: new RegExp(`${defaultStartLabel}.*${defaultEndLabel}`),
		});

		await userEvent.click(dateRangeTrigger);
		const last7Days = await body.findByRole("button", {
			name: "Last 7 days",
		});

		await userEvent.click(last7Days);

		await waitFor(() => {
			expect(body.queryByRole("button", { name: "Last 7 days" })).toBeNull();
			const calls = getChatCostUsersCalls();
			expect(calls.length).toBeGreaterThan(initialCallCount);

			const latestCall = calls.at(-1)?.[0];
			expect(latestCall).toBeDefined();
			if (!latestCall) {
				throw new Error("Expected getChatCostUsers to be called with params.");
			}

			expect(latestCall.start_date).not.toBe(defaultStartDate);
		});
	},
};

export const UsageDateFilterRefetchOverlay: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		let requestCount = 0;
		let resolveRefetch:
			| ((value: TypesGen.ChatCostUsersResponse) => void)
			| undefined;
		const refetchPromise = new Promise<TypesGen.ChatCostUsersResponse>(
			(resolve) => {
				resolveRefetch = resolve;
			},
		);

		spyOn(API.experimental, "getChatCostUsers").mockImplementation(async () => {
			requestCount += 1;
			if (requestCount === 1) {
				return mockUsersResponse;
			}

			return refetchPromise;
		});
		spyOn(API, "getUser").mockResolvedValue(mockUserProfile);
		spyOn(API.experimental, "getChatCostSummary").mockResolvedValue(
			mockCostSummary,
		);

		return () => {
			resolveRefetch?.({
				...mockUsersResponse,
				start_date: "2026-03-06T00:00:00Z",
				end_date: "2026-03-12T00:00:00Z",
			});
		};
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const defaultStartLabel = fixedNow
			.subtract(30, "day")
			.format("MMM D, YYYY");
		const defaultEndLabel = fixedNow.format("MMM D, YYYY");

		await canvas.findByText("Alice Liddell");

		await step(
			"show a refetch overlay after changing the date range",
			async () => {
				const dateRangeTrigger = await canvas.findByRole("button", {
					name: new RegExp(`${defaultStartLabel}.*${defaultEndLabel}`),
				});

				await userEvent.click(dateRangeTrigger);
				await userEvent.click(
					await body.findByRole("button", { name: "Last 7 days" }),
				);

				await expect(
					await canvas.findByRole("status", { name: "Refreshing usage" }),
				).toBeInTheDocument();
			},
		);
	},
};

export const UsageEmpty: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	beforeEach: () => {
		setupUsageSpies({
			usersResponse: {
				start_date: "2026-02-10T00:00:00Z",
				end_date: "2026-03-12T00:00:00Z",
				count: 0,
				users: [],
			},
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Usage");
		await expect(
			await canvas.findByText("No usage data for this period."),
		).toBeInTheDocument();
	},
};

export const UsageUserDrillIn: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	parameters: {
		queries: [{ key: userKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Alice's row to drill into the detail view.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// Wait for the detail view to mount. "User ID:" only
		// renders in the detail panel, not the list.
		await body.findByText(`User ID: ${mockUserProfile.id}`);

		await expect(body.getByText("Alice Liddell")).toBeInTheDocument();
		await expect(body.getByText("@alice")).toBeInTheDocument();

		// The cost summary should have been fetched.
		await waitFor(() => {
			expect(API.experimental.getChatCostSummary).toHaveBeenCalled();
		});

		// The Back button should be visible.
		await expect(body.getByText("Back")).toBeInTheDocument();
	},
};

export const UsageUserDrillInAndBack: Story = {
	args: {
		activeSection: "usage",
		canManageChatModelConfigs: true,
	},
	parameters: {
		queries: [{ key: userKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Alice's row to drill into the detail view.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// Wait for the detail view to mount. "User ID:" only
		// renders in the detail panel, not the list.
		await body.findByText(`User ID: ${mockUserProfile.id}`);

		// Click Back to return to the list.
		await userEvent.click(body.getByText("Back"));

		// The user list should be visible again with both users.
		await expect(await body.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(body.getByText("Bob Builder")).toBeInTheDocument();

		// The search field should be present, confirming we're
		// back on the list view.
		await expect(
			body.getByPlaceholderText("Search by name or username"),
		).toBeInTheDocument();
	},
};

// ── Invisible Unicode warning stories ──────────────────────────

export const InvisibleUnicodeWarningSystemPrompt: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatSystemPrompt").mockResolvedValue({
			system_prompt:
				"Normal prompt text\u200b\u200b\u200b\u200bhidden instruction",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the System Instructions section to render.
		await canvas.findByText("System Instructions");

		// The warning alert should appear with the correct count.
		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("4");
	},
};

export const InvisibleUnicodeWarningUserPrompt: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "My custom prompt\u200b\u200c\u200dhidden",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the Personal Instructions section to render.
		await canvas.findByText("Personal Instructions");

		// The warning alert should appear.
		const alert = await canvas.findByText(/invisible Unicode/);
		expect(alert).toBeInTheDocument();
		expect(alert.textContent).toContain("2");
	},
};

export const InvisibleUnicodeWarningOnType: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for the Personal Instructions textarea to render.
		const textarea = await canvas.findByPlaceholderText(
			"Additional behavior, style, and tone preferences",
		);

		// No warning should be present initially.
		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();

		// Type a string containing a ZWS character.
		await userEvent.type(textarea, "hello\u200bworld");

		// The warning alert should appear dynamically.
		await waitFor(() => {
			expect(canvas.getByText(/invisible Unicode/)).toBeInTheDocument();
		});
	},
};

export const NoWarningForCleanPrompt: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "You are a helpful coding assistant.",
		});
		spyOn(API.experimental, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "Be concise and use TypeScript.",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		// Wait for both sections to render.
		await canvas.findByText("Personal Instructions");
		await canvas.findByText("System Instructions");

		// No invisible Unicode warning should be present.
		expect(canvas.queryByText(/invisible Unicode/)).toBeNull();
	},
};

// ── Templates tab stories ──────────────────────────────────────

const manyTemplates = [
	{ id: "t-01", name: "docker-dev", display_name: "Docker Development" },
	{
		id: "t-02",
		name: "kubernetes-prod",
		display_name: "Kubernetes Production",
	},
	{ id: "t-03", name: "aws-windows", display_name: "AWS Windows Desktop" },
	{ id: "t-04", name: "gcp-linux", display_name: "GCP Linux Workspace" },
	{ id: "t-05", name: "azure-dotnet", display_name: "Azure .NET Environment" },
	{ id: "t-06", name: "ml-jupyter", display_name: "ML Jupyter Notebook" },
	{
		id: "t-07",
		name: "data-eng-spark",
		display_name: "Data Engineering (Spark)",
	},
	{
		id: "t-08",
		name: "frontend-vite",
		display_name: "Frontend (Vite + React)",
	},
].map((t) => ({ ...MockTemplate, ...t }));

export const TemplateAllowlist: Story = {
	args: {
		activeSection: "templates",
		canManageChatModelConfigs: true,
		canSetSystemPrompt: true,
	},
	beforeEach: () => {
		// Track saved allowlist state across mock calls so the
		// refetch after save returns the updated value.
		let savedIDs: string[] = [];

		spyOn(API, "getTemplates").mockResolvedValue(manyTemplates);
		spyOn(API.experimental, "getChatTemplateAllowlist").mockImplementation(
			async () => ({ template_ids: savedIDs }),
		);
		spyOn(API.experimental, "updateChatTemplateAllowlist").mockImplementation(
			async (req) => {
				savedIDs = [...req.template_ids];
			},
		);
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("starts empty", async () => {
			// Status text confirms no restrictions.
			await canvas.findByText(/no templates selected/i);
			// Save is disabled — nothing to save.
			const saveBtn = await canvas.findByRole("button", { name: "Save" });
			expect(saveBtn).toBeDisabled();
		});

		await step("select one template and save", async () => {
			// Open the combobox.
			const input = canvas.getByPlaceholderText("Select templates...");
			await userEvent.click(input);
			// Pick the first template from the dropdown.
			await userEvent.click(
				await canvas.findByRole("option", { name: "Docker Development" }),
			);
			// Badge pill should appear and status should update.
			await waitFor(() => {
				expect(canvas.getByText("1 template selected")).toBeInTheDocument();
			});
			// Save should now be enabled.
			const saveBtn = canvas.getByRole("button", { name: "Save" });
			expect(saveBtn).toBeEnabled();
			await userEvent.click(saveBtn);
			await waitFor(() => {
				expect(
					API.experimental.updateChatTemplateAllowlist,
				).toHaveBeenCalledWith({ template_ids: ["t-01"] });
			});
		});

		await step("add the remaining seven and save", async () => {
			// Open the combobox again.
			const input = canvas.getByLabelText("Select allowed templates");
			await userEvent.click(input);
			// Select the other seven templates one by one.
			for (const name of [
				"Kubernetes Production",
				"AWS Windows Desktop",
				"GCP Linux Workspace",
				"Azure .NET Environment",
				"ML Jupyter Notebook",
				"Data Engineering (Spark)",
				"Frontend (Vite + React)",
			]) {
				await userEvent.click(await canvas.findByRole("option", { name }));
			}
			// All eight should now be selected.
			await waitFor(() => {
				expect(canvas.getByText("8 templates selected")).toBeInTheDocument();
			});
			// Save.
			const saveBtn = canvas.getByRole("button", { name: "Save" });
			await userEvent.click(saveBtn);
			await waitFor(() => {
				expect(
					API.experimental.updateChatTemplateAllowlist,
				).toHaveBeenLastCalledWith({
					template_ids: expect.arrayContaining([
						"t-01",
						"t-02",
						"t-03",
						"t-04",
						"t-05",
						"t-06",
						"t-07",
						"t-08",
					]),
				});
			});
		});
	},
};
