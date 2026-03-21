import { MockUserOwner } from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { userByNameKey } from "api/queries/users";
import type * as TypesGen from "api/typesGenerated";
import dayjs from "dayjs";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { SettingsPageContent } from "./SettingsPageContent";

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
	spyOn(API, "getChatCostUsers").mockResolvedValue(
		opts?.usersResponse ?? mockUsersResponse,
	);
	spyOn(API, "getUser").mockResolvedValue(mockUserProfile);
	spyOn(API, "getChatCostSummary").mockResolvedValue(mockCostSummary);
};

const getChatCostUsersCalls = () =>
	(
		API.getChatCostUsers as typeof API.getChatCostUsers & {
			mock: {
				calls: Array<[Parameters<typeof API.getChatCostUsers>[0]]>;
			};
		}
	).mock.calls;

const fixedNow = dayjs("2026-03-12T00:00:00Z");

// ── Meta ───────────────────────────────────────────────────────

const meta = {
	title: "pages/AgentsPage/SettingsPageContent",
	component: SettingsPageContent,
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
		spyOn(API, "getChatSystemPrompt").mockResolvedValue({
			system_prompt: "",
		});
		spyOn(API, "updateChatSystemPrompt").mockResolvedValue();
		spyOn(API, "getChatDesktopEnabled").mockResolvedValue({
			enable_desktop: false,
		});
		spyOn(API, "updateChatDesktopEnabled").mockResolvedValue();
		spyOn(API, "getUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API, "updateUserChatCustomPrompt").mockResolvedValue({
			custom_prompt: "",
		});
		spyOn(API, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 0,
		});
		spyOn(API, "updateChatWorkspaceTTL").mockResolvedValue();
	},
} satisfies Meta<typeof SettingsPageContent>;

export default meta;
type Story = StoryObj<typeof SettingsPageContent>;

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
			expect(API.updateChatDesktopEnabled).toHaveBeenCalledWith({
				enable_desktop: true,
			});
		});
	},
};

export const DefaultAutostopDefault: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await canvas.findByText("Default Autostop");
		// When disabled (0s), shows template-default copy.
		await canvas.findByText(/stopped as configured by their templates/i);

		// DurationField renders a text input labeled "Default autostop".
		const durationInput = await canvas.findByLabelText("Default autostop");

		// Default is "0s" → 0 hours (disabled).
		expect(durationInput).toHaveValue("0");

		// Save button should be disabled (no local change).
		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });
		expect(saveButton).toBeDisabled();
	},
};

export const DefaultAutostopCustomValue: Story = {
	beforeEach: () => {
		// 2h = 2 hours exactly, shows cleanly in DurationField.
		spyOn(API, "getChatWorkspaceTTL").mockResolvedValue({
			workspace_ttl_ms: 7_200_000,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const durationInput = await canvas.findByLabelText("Default autostop");

		// Shows 2 hours from the mock.
		expect(durationInput).toHaveValue("2");

		// When non-zero, shows the duration in the description.
		await canvas.findByText(/stopped after 2 hours of inactivity/i);
	},
};

export const DefaultAutostopSave: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const durationInput = await canvas.findByLabelText("Default autostop");
		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });

		// Change to 3 hours.
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "3");

		// Save button should now be enabled.
		await waitFor(() => {
			expect(saveButton).toBeEnabled();
		});

		await userEvent.click(saveButton);
		await waitFor(() => {
			expect(API.updateChatWorkspaceTTL).toHaveBeenCalledWith({
				workspace_ttl_ms: 10_800_000,
			});
		});
	},
};

export const DefaultAutostopExceedsMax: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const durationInput = await canvas.findByLabelText("Default autostop");
		const ttlForm = durationInput.closest("form")!;
		const saveButton = within(ttlForm).getByRole("button", { name: "Save" });

		// Enter 721 hours (exceeds 30-day / 720h limit).
		await userEvent.clear(durationInput);
		await userEvent.type(durationInput, "721");

		// Error helper text should appear.
		await waitFor(() => {
			expect(canvas.getByText(/must not exceed 30 days/i)).toBeInTheDocument();
		});

		// Save button should be disabled despite the field being dirty.
		expect(saveButton).toBeDisabled();
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
		const ttlHeading = canvas.queryByText("Default Autostop");
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
			expect(API.getChatCostUsers).toHaveBeenCalled();
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

		spyOn(API, "getChatCostUsers").mockImplementation(async () => {
			requestCount += 1;
			if (requestCount === 1) {
				return mockUsersResponse;
			}

			return refetchPromise;
		});
		spyOn(API, "getUser").mockResolvedValue(mockUserProfile);
		spyOn(API, "getChatCostSummary").mockResolvedValue(mockCostSummary);

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
		queries: [{ key: userByNameKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Wait for the user list to load.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// The detail view should show the user header with name
		// and username subtitle.
		await expect(await body.findByText("Alice Liddell")).toBeInTheDocument();
		await expect(body.getByText("@alice")).toBeInTheDocument();

		// The user profile was pre-seeded in the query cache via
		// parameters.queries, so the detail header should show the
		// user ID from that data.
		await expect(
			body.getByText(`User ID: ${mockUserProfile.id}`),
		).toBeInTheDocument();

		// The cost summary should have been fetched.
		await waitFor(() => {
			expect(API.getChatCostSummary).toHaveBeenCalled();
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
		queries: [{ key: userByNameKey("user-1"), data: mockUserProfile }],
	},
	beforeEach: () => {
		setupUsageSpies();
	},
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);

		// Click Alice's row to drill in.
		await userEvent.click(await body.findByText("Alice Liddell"));

		// Wait for the detail view to appear.
		await body.findByText("@alice");

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
