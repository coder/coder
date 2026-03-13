import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type * as TypesGen from "api/typesGenerated";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	ConfigureAgentsDialog,
	type ConfigureAgentsSection,
} from "./ConfigureAgentsDialog";

const mockChatCostUsers = vi.fn();
const mockChatCostSummary = vi.fn();
const mockUpdateChatSystemPrompt = vi.fn();
const mockUpdateUserChatCustomPrompt = vi.fn();

vi.mock("api/queries/chats", () => ({
	chatCostUsers: (params?: unknown) => ({
		queryKey: ["chatCostUsers", params],
		queryFn: () => mockChatCostUsers(params),
	}),
	chatCostSummary: (user = "me", params?: unknown) => ({
		queryKey: ["chatCostSummary", user, params],
		queryFn: () => mockChatCostSummary(user, params),
	}),
	chatSystemPrompt: () => ({
		queryKey: ["chatSystemPrompt"],
		queryFn: () => Promise.resolve({ system_prompt: "" }),
	}),
	chatUserCustomPrompt: () => ({
		queryKey: ["chatUserCustomPrompt"],
		queryFn: () => Promise.resolve({ custom_prompt: "" }),
	}),
	updateChatSystemPrompt: () => ({
		mutationFn: mockUpdateChatSystemPrompt,
	}),
	updateUserChatCustomPrompt: () => ({
		mutationFn: mockUpdateUserChatCustomPrompt,
	}),
}));

vi.mock("components/Dialog/Dialog", () => ({
	Dialog: ({ open, children }: { open: boolean; children: ReactNode }) =>
		open ? <div role="dialog">{children}</div> : null,
	DialogClose: ({ children }: { children: ReactNode }) => <>{children}</>,
	DialogContent: ({
		children,
		className,
	}: {
		children: ReactNode;
		className?: string;
	}) => <div className={className}>{children}</div>,
	DialogDescription: ({ children }: { children: ReactNode }) => (
		<p>{children}</p>
	),
	DialogHeader: ({
		children,
		className,
	}: {
		children: ReactNode;
		className?: string;
	}) => <div className={className}>{children}</div>,
	DialogTitle: ({ children }: { children: ReactNode }) => <h2>{children}</h2>,
}));

vi.mock("components/Tooltip/Tooltip", () => ({
	Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
	TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
	TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
	TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

vi.mock("./ChatModelAdminPanel/ChatModelAdminPanel", () => ({
	ChatModelAdminPanel: () => <div data-testid="chat-model-admin-panel" />,
}));

vi.mock("components/PaginationWidget/PaginationAmount", () => ({
	PaginationAmount: ({ totalRecords }: { totalRecords: number }) => (
		<div data-testid="pagination-amount">{totalRecords} users</div>
	),
}));

vi.mock("components/PaginationWidget/PaginationWidgetBase", () => ({
	PaginationWidgetBase: () => <div data-testid="pagination-widget" />,
}));

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

const buildUser = (
	overrides: Partial<TypesGen.ChatCostUserRollup> = {},
): TypesGen.ChatCostUserRollup => ({
	user_id: "user-1",
	username: "alice",
	name: "Alice Example",
	avatar_url: "https://example.com/alice.png",
	total_cost_micros: 1_200_000,
	message_count: 12,
	chat_count: 3,
	total_input_tokens: 120_000,
	total_output_tokens: 45_000,
	...overrides,
});

const usersResponse: TypesGen.ChatCostUsersResponse = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	count: 2,
	users: [
		buildUser(),
		buildUser({
			user_id: "user-2",
			username: "bob",
			name: "Bob Example",
			avatar_url: "https://example.com/bob.png",
			total_cost_micros: 900_000,
			message_count: 8,
			chat_count: 2,
			total_input_tokens: 80_000,
			total_output_tokens: 30_000,
		}),
	],
};

const summaryResponse: TypesGen.ChatCostSummary = {
	start_date: "2026-02-10T00:00:00Z",
	end_date: "2026-03-12T00:00:00Z",
	total_cost_micros: 1_200_000,
	priced_message_count: 12,
	unpriced_message_count: 0,
	total_input_tokens: 120_000,
	total_output_tokens: 45_000,
	by_model: [],
	by_chat: [],
};

const renderDialog = (initialSection: ConfigureAgentsSection = "usage") => {
	const queryClient = createQueryClient();
	return render(
		<QueryClientProvider client={queryClient}>
			<ConfigureAgentsDialog
				open
				onOpenChange={vi.fn()}
				canManageChatModelConfigs
				canSetSystemPrompt={false}
				initialSection={initialSection}
			/>
		</QueryClientProvider>,
	);
};

describe("ConfigureAgentsDialog usage tab", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockChatCostUsers.mockResolvedValue(usersResponse);
		mockChatCostSummary.mockResolvedValue(summaryResponse);
		mockUpdateChatSystemPrompt.mockResolvedValue(undefined);
		mockUpdateUserChatCustomPrompt.mockResolvedValue({ custom_prompt: "" });
	});

	it("renders usage table user rows", async () => {
		renderDialog();

		expect(await screen.findByText("Alice Example")).toBeInTheDocument();
		expect(screen.getByText("Bob Example")).toBeInTheDocument();
		expect(screen.getByText("Total Cost")).toBeInTheDocument();
	});

	it("shows the detail pane when a user row is clicked", async () => {
		const user = userEvent.setup();
		renderDialog();

		const row = (await screen.findByText("Alice Example")).closest("tr");
		expect(row).not.toBeNull();

		await user.click(row!);

		expect(
			await screen.findByRole("button", { name: /back to all users/i }),
		).toBeInTheDocument();
		expect(screen.getByText("User ID: user-1")).toBeInTheDocument();
	});

	it("shows the detail pane when Enter is pressed on a user row", async () => {
		renderDialog();

		const row = (await screen.findByText("Alice Example")).closest("tr");
		expect(row).not.toBeNull();
		row!.focus();

		fireEvent.keyDown(row!, { key: "Enter" });

		expect(
			await screen.findByRole("button", { name: /back to all users/i }),
		).toBeInTheDocument();
		expect(screen.getByText("User ID: user-1")).toBeInTheDocument();
	});

	it("shows the usage error state with a retry button", async () => {
		const user = userEvent.setup();
		mockChatCostUsers.mockRejectedValue("Failed to load usage data.");
		renderDialog("behavior");

		await user.click(screen.getByRole("button", { name: /usage/i }));

		await waitFor(() => expect(mockChatCostUsers).toHaveBeenCalled());
		expect(
			await screen.findByText("Failed to load usage data."),
		).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
	});
});
