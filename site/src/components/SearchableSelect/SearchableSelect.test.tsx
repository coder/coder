import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
	SearchableSelect,
	SearchableSelectContent,
	SearchableSelectItem,
	SearchableSelectTrigger,
	SearchableSelectValue,
} from "../SearchableSelect";

describe("SearchableSelect", () => {
	it("renders with placeholder", () => {
		render(
			<SearchableSelect placeholder="Select an option">
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		expect(screen.getByText("Select an option")).toBeInTheDocument();
	});

	it("opens dropdown when trigger is clicked", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
					<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		expect(screen.getByPlaceholderText("Search...")).toBeInTheDocument();
		expect(screen.getByText("Option 1")).toBeInTheDocument();
		expect(screen.getByText("Option 2")).toBeInTheDocument();
	});

	it("filters options based on search input", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="apple">Apple</SearchableSelectItem>
					<SearchableSelectItem value="banana">Banana</SearchableSelectItem>
					<SearchableSelectItem value="cherry">Cherry</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const searchInput = screen.getByPlaceholderText("Search...");
		await user.type(searchInput, "ban");

		expect(screen.getByText("Banana")).toBeInTheDocument();
		expect(screen.queryByText("Apple")).not.toBeInTheDocument();
		expect(screen.queryByText("Cherry")).not.toBeInTheDocument();
	});

	it("selects an option when clicked", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();

		render(
			<SearchableSelect onValueChange={onValueChange}>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
					<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const option2 = screen.getByText("Option 2");
		await user.click(option2);

		expect(onValueChange).toHaveBeenCalledWith("option2");

		// Dropdown should close after selection
		await waitFor(() => {
			expect(
				screen.queryByPlaceholderText("Search..."),
			).not.toBeInTheDocument();
		});
	});

	it("displays selected value", () => {
		render(
			<SearchableSelect value="option2">
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
					<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		expect(screen.getByRole("combobox")).toHaveTextContent("Option 2");
	});

	it("shows check mark for selected option", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect value="option2">
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
					<SearchableSelectItem value="option2">Option 2</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		// The selected item should have a check mark (SVG element)
		const option2Item = screen.getByText("Option 2").closest('[role="option"]');
		const checkIcon = option2Item?.querySelector("svg");
		expect(checkIcon).toBeInTheDocument();
	});

	it("shows empty message when no results match search", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect emptyMessage="No items found">
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="apple">Apple</SearchableSelectItem>
					<SearchableSelectItem value="banana">Banana</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const searchInput = screen.getByPlaceholderText("Search...");
		await user.type(searchInput, "xyz");

		expect(screen.getByText("No items found")).toBeInTheDocument();
	});

	it("clears search when dropdown closes", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="apple">Apple</SearchableSelectItem>
					<SearchableSelectItem value="banana">Banana</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const searchInput = screen.getByPlaceholderText("Search...");
		await user.type(searchInput, "ban");

		// Close by clicking outside
		await user.click(document.body);

		// Reopen
		await user.click(trigger);

		// All options should be visible again
		expect(screen.getByText("Apple")).toBeInTheDocument();
		expect(screen.getByText("Banana")).toBeInTheDocument();
	});

	it("respects disabled state", async () => {
		const user = userEvent.setup();
		const onValueChange = vi.fn();

		render(
			<SearchableSelect disabled onValueChange={onValueChange}>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		expect(trigger).toBeDisabled();

		await user.click(trigger);
		expect(screen.queryByPlaceholderText("Search...")).not.toBeInTheDocument();
		expect(onValueChange).not.toHaveBeenCalled();
	});

	it("supports custom id", () => {
		render(
			<SearchableSelect id="my-select">
				<SearchableSelectTrigger id="my-trigger">
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">Option 1</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		expect(document.getElementById("my-select")).toBeInTheDocument();
		expect(document.getElementById("my-trigger")).toBeInTheDocument();
	});

	it("filters by option value when text doesn't match", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="us-east-1">
						US East (N. Virginia)
					</SearchableSelectItem>
					<SearchableSelectItem value="eu-west-1">
						EU (Ireland)
					</SearchableSelectItem>
					<SearchableSelectItem value="ap-southeast-1">
						Asia Pacific (Singapore)
					</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const searchInput = screen.getByPlaceholderText("Search...");
		await user.type(searchInput, "west");

		// Should find the EU option by its value
		expect(screen.getByText("EU (Ireland)")).toBeInTheDocument();
		expect(screen.queryByText("US East (N. Virginia)")).not.toBeInTheDocument();
		expect(
			screen.queryByText("Asia Pacific (Singapore)"),
		).not.toBeInTheDocument();
	});

	it("supports complex content in items", async () => {
		const user = userEvent.setup();

		render(
			<SearchableSelect>
				<SearchableSelectTrigger>
					<SearchableSelectValue />
				</SearchableSelectTrigger>
				<SearchableSelectContent>
					<SearchableSelectItem value="option1">
						<div className="flex items-center gap-2">
							<span className="icon">🍎</span>
							<span>Apple</span>
						</div>
					</SearchableSelectItem>
					<SearchableSelectItem value="option2">
						<div className="flex items-center gap-2">
							<span className="icon">🍌</span>
							<span>Banana</span>
						</div>
					</SearchableSelectItem>
				</SearchableSelectContent>
			</SearchableSelect>,
		);

		const trigger = screen.getByRole("combobox");
		await user.click(trigger);

		const searchInput = screen.getByPlaceholderText("Search...");
		await user.type(searchInput, "apple");

		// Should still find Apple even with complex structure
		expect(screen.getByText("Apple")).toBeInTheDocument();
		expect(screen.queryByText("Banana")).not.toBeInTheDocument();
	});
});
