import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { render } from "#/testHelpers/renderHelpers";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterCategory } from "./types";

const ownerCategory: FilterCategory = {
	key: "owner",
	label: "Owner",
	getOptions: async () => [{ label: "alice", value: "alice" }],
};

const statusCategory: FilterCategory = {
	key: "status",
	label: "Status",
	inlineOptions: true,
	getOptions: async () => [{ label: "Running", value: "running" }],
};

const attributesCategory: FilterCategory = {
	key: "attribute",
	label: "Attributes",
	chipKeys: ["outdated", "shared"],
	inlineOptions: true,
	inlineOptionsExclusive: true,
	inlineOptionsLabel: "Workspace is…",
	getOptions: async () => [
		{ label: "Outdated", value: "outdated", token: "outdated:true" },
		{ label: "Shared", value: "shared", token: "shared:true" },
	],
};

type FilterComboboxHarnessProps = {
	categories: readonly FilterCategory[];
	initialValue?: string;
	onChange: (value: string) => void;
};

function FilterComboboxHarness({
	categories,
	initialValue = "",
	onChange,
}: FilterComboboxHarnessProps) {
	const [value, setValue] = useState(initialValue);

	return (
		<FilterCombobox
			value={value}
			onChange={(nextValue) => {
				onChange(nextValue);
				setValue(nextValue);
			}}
			categories={categories}
			placeholder="Search and filter"
		/>
	);
}

describe("FilterCombobox", () => {
	it("opens from the Filters button with keyboard focus and navigates categories", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<FilterComboboxHarness
				categories={[ownerCategory]}
				onChange={onChange}
			/>,
		);

		const filtersButton = screen.getByRole("button", {
			name: "Toggle filters",
		});
		const input = screen.getByRole("combobox", {
			name: "Search and filter",
		});
		await user.tab();
		expect(filtersButton).toHaveFocus();

		await user.keyboard("{Enter}");
		expect(input).toHaveFocus();
		await user.keyboard("{ArrowDown}{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("emits unmatched typed text without selecting a filter", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<FilterComboboxHarness
				categories={[
					{
						...ownerCategory,
						getOptions: async (query) =>
							query === "missing" ? [] : ownerCategory.getOptions(query),
					},
				]}
				onChange={onChange}
			/>,
		);

		const input = screen.getByRole("combobox", { name: "Search and filter" });
		await user.click(input);
		await user.type(input, "missing");

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("missing"));
	});

	it("replaces the selected Workspace attribute", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<FilterComboboxHarness
				categories={[attributesCategory]}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Toggle filters" }));
		await user.click(await screen.findByRole("option", { name: "Outdated" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("outdated:true"),
		);
		await user.click(await screen.findByRole("option", { name: "Shared" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("shared:true"),
		);
	});

	it("keeps the main filter menu usable after selecting a flyout option", async () => {
		const user = userEvent.setup({ skipHover: true });
		const onChange = vi.fn();
		render(
			<FilterComboboxHarness
				categories={[ownerCategory, statusCategory]}
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Toggle filters" }));
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice status:running"),
		);
	});

	it("removes a selected inline option", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<FilterComboboxHarness
				categories={[statusCategory]}
				initialValue="status:running"
				onChange={onChange}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Toggle filters" }));
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
	});
});
