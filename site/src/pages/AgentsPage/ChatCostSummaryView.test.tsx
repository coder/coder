import { render, screen } from "@testing-library/react";
import type * as TypesGen from "api/typesGenerated";
import { describe, expect, it, vi } from "vitest";
import { ChatCostSummaryView } from "./ChatCostSummaryView";

const buildSummary = (
	overrides: Partial<TypesGen.ChatCostSummary> = {},
): TypesGen.ChatCostSummary => ({
	start_date: "2026-02-10",
	end_date: "2026-03-12",
	total_cost_micros: 1_500_000,
	priced_message_count: 12,
	unpriced_message_count: 0,
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

const defaultProps = {
	summary: undefined,
	isLoading: false,
	isError: false,
	error: undefined,
	onRetry: vi.fn(),
	loadingLabel: "Loading usage details",
	emptyMessage: "No usage details available.",
};

describe("ChatCostSummaryView", () => {
	it("renders the loading state with the provided label", () => {
		render(<ChatCostSummaryView {...defaultProps} isLoading />);

		expect(screen.getByRole("status")).toHaveAttribute(
			"aria-label",
			"Loading usage details",
		);
	});

	it("renders the default error message for non-Error values", () => {
		render(<ChatCostSummaryView {...defaultProps} isError error="string" />);

		expect(
			screen.getByText("Failed to load usage details."),
		).toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
	});

	it("renders Error messages when available", () => {
		render(
			<ChatCostSummaryView
				{...defaultProps}
				isError
				error={new Error("Custom")}
			/>,
		);

		expect(screen.getByText("Custom")).toBeInTheDocument();
	});

	it("renders the empty state when no summary rows are available", () => {
		render(
			<ChatCostSummaryView
				{...defaultProps}
				summary={buildSummary({ by_model: [], by_chat: [] })}
			/>,
		);

		expect(screen.getByText("No usage details available.")).toBeInTheDocument();
	});

	it("renders summary cards and table content", () => {
		render(<ChatCostSummaryView {...defaultProps} summary={buildSummary()} />);

		expect(screen.getByText("Total Cost")).toBeInTheDocument();
		expect(screen.getByText("Input Tokens")).toBeInTheDocument();
		expect(screen.getByText("Output Tokens")).toBeInTheDocument();
		expect(screen.getAllByText("Messages").length).toBeGreaterThan(0);
		expect(screen.getByText("GPT-4.1")).toBeInTheDocument();
		expect(screen.getByText("Quarterly review")).toBeInTheDocument();
	});

	it("renders the unpriced warning when summary data is incomplete", () => {
		render(
			<ChatCostSummaryView
				{...defaultProps}
				summary={buildSummary({ unpriced_message_count: 2 })}
			/>,
		);

		expect(
			screen.getByText(
				"2 messages could not be priced because model pricing data was unavailable.",
			),
		).toBeInTheDocument();
	});
});
