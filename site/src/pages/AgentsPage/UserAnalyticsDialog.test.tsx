import { render, screen, waitFor } from "@testing-library/react";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { AuthContext, type AuthContextValue } from "contexts/auth/AuthProvider";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { UserAnalyticsDialog } from "./UserAnalyticsDialog";

vi.mock("api/api", () => ({
	API: {
		getChatCostSummary: vi.fn(),
	},
}));

const mockGetChatCostSummary = vi.mocked(API.getChatCostSummary);

const createQueryClient = () =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				refetchOnWindowFocus: false,
				gcTime: 0,
			},
		},
	});

const authValue: AuthContextValue = {
	isLoading: false,
	isSignedOut: false,
	isSigningOut: false,
	isConfiguringTheFirstUser: false,
	isSignedIn: true,
	isSigningIn: false,
	isUpdatingProfile: false,
	user: {
		id: "user-123",
		username: "alice",
		name: "Alice Example",
		avatar_url: "https://example.com/avatar.png",
		email: "alice@example.com",
		login_type: "password",
		created_at: "2026-01-01T00:00:00Z",
		updated_at: "2026-01-01T00:00:00Z",
		status: "active",
		roles: [],
		organization_ids: [],
		last_seen_at: "2026-03-12T00:00:00Z",
		quiet_hours_schedule: "",
	} as TypesGen.User,
	permissions: undefined,
	signInError: undefined,
	updateProfileError: undefined,
	signOut: vi.fn(),
	signIn: vi.fn(),
	updateProfile: vi.fn(),
};

const buildSummary = (
	overrides: Partial<TypesGen.ChatCostSummary> = {},
): TypesGen.ChatCostSummary => ({
	start_date: "2026-02-10",
	end_date: "2026-03-12",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 1,
	total_input_tokens: 123_456,
	total_output_tokens: 654_321,
	by_model: [
		{
			model_config_id: "model-config-1",
			display_name: "GPT-4.1",
			provider: "OpenAI",
			model: "gpt-4.1",
			total_cost_micros: 1_250_000,
			message_count: 9,
			total_input_tokens: 100_000,
			total_output_tokens: 200_000,
		},
	],
	by_chat: [
		{
			root_chat_id: "chat-1",
			chat_title: "Quarterly review",
			total_cost_micros: 750_000,
			message_count: 5,
			total_input_tokens: 60_000,
			total_output_tokens: 80_000,
		},
	],
	...overrides,
});

const renderDialog = () => {
	const queryClient = createQueryClient();
	return render(
		<AuthContext.Provider value={authValue}>
			<QueryClientProvider client={queryClient}>
				<UserAnalyticsDialog open onOpenChange={vi.fn()} />
			</QueryClientProvider>
		</AuthContext.Provider>,
	);
};

describe("UserAnalyticsDialog", () => {
	beforeEach(() => {
		mockGetChatCostSummary.mockReset();
	});

	it("loads the current user's analytics in a modal", async () => {
		mockGetChatCostSummary.mockResolvedValue(buildSummary());

		renderDialog();

		await waitFor(() => {
			expect(mockGetChatCostSummary).toHaveBeenCalledWith(
				"user-123",
				expect.objectContaining({
					start_date: expect.any(String),
					end_date: expect.any(String),
				}),
			);
		});

		expect(await screen.findByRole("dialog")).toBeInTheDocument();
		expect(screen.getByText("$1.50")).toBeInTheDocument();
		expect(screen.getByText("GPT-4.1")).toBeInTheDocument();
		expect(screen.getByText("Quarterly review")).toBeInTheDocument();
	});
});
