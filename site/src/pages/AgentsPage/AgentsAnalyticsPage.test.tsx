import { render, screen, waitFor, within } from "@testing-library/react";
import { API } from "api/api";
import type * as TypesGen from "api/typesGenerated";
import { QueryClient, QueryClientProvider } from "react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AgentsAnalyticsPage from "./AgentsAnalyticsPage";

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

const buildSummary = (
	overrides: Partial<TypesGen.ChatCostSummary> = {},
): TypesGen.ChatCostSummary => ({
	start_date: "2026-02-10",
	end_date: "2026-03-12",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 2,
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

const renderPage = () => {
	const queryClient = createQueryClient();

	return render(
		<QueryClientProvider client={queryClient}>
			<AgentsAnalyticsPage />
		</QueryClientProvider>,
	);
};

describe("AgentsAnalyticsPage", () => {
	beforeEach(() => {
		mockGetChatCostSummary.mockReset();
	});

	it("renders loading state", () => {
		mockGetChatCostSummary.mockImplementation(
			() => new Promise(() => undefined),
		);

		renderPage();

		expect(
			screen.getByRole("status", { name: /loading analytics/i }),
		).toBeInTheDocument();
	});

	it("renders summary data", async () => {
		mockGetChatCostSummary.mockResolvedValue(buildSummary());

		renderPage();

		await waitFor(() => {
			expect(screen.getByText("$1.50")).toBeInTheDocument();
		});

		expect(screen.getByText("123,456")).toBeInTheDocument();
		expect(screen.getByText("654,321")).toBeInTheDocument();
		expect(screen.getByText("14")).toBeInTheDocument();
	});

	it("shows unpriced banner when unpriced_message_count > 0", async () => {
		mockGetChatCostSummary.mockResolvedValue(
			buildSummary({ unpriced_message_count: 3 }),
		);

		renderPage();

		await waitFor(() => {
			expect(screen.getByTestId("unpriced-banner")).toBeInTheDocument();
		});
	});

	it("hides unpriced banner when unpriced_message_count is 0", async () => {
		mockGetChatCostSummary.mockResolvedValue(
			buildSummary({ unpriced_message_count: 0 }),
		);

		renderPage();

		await waitFor(() => {
			expect(screen.getByText("$1.50")).toBeInTheDocument();
		});

		expect(screen.queryByTestId("unpriced-banner")).not.toBeInTheDocument();
	});

	it("renders model breakdown table", async () => {
		mockGetChatCostSummary.mockResolvedValue(buildSummary());

		renderPage();

		const modelBreakdown = await screen.findByTestId("model-breakdown");
		const modelTable = within(modelBreakdown);

		expect(modelTable.getByText("GPT-4.1")).toBeInTheDocument();
		expect(modelTable.getByText("OpenAI")).toBeInTheDocument();
		expect(modelTable.getByText("$1.25")).toBeInTheDocument();
	});

	it("renders chat breakdown table", async () => {
		mockGetChatCostSummary.mockResolvedValue(buildSummary());

		renderPage();

		const chatBreakdown = await screen.findByTestId("chat-breakdown");
		const chatTable = within(chatBreakdown);

		expect(chatTable.getByText("Quarterly review")).toBeInTheDocument();
		expect(chatTable.getByText("$0.75")).toBeInTheDocument();
		expect(chatTable.getByText("80,000")).toBeInTheDocument();
	});
});
