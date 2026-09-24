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

const scopedOwnerCategory: FilterCategory = {
	...ownerCategory,
	chipKeys: ["owner", "user"],
	scopeToggle: {
		label: (owner) =>
			owner
				? `Include workspaces shared with ${owner}`
				: "Include shared workspaces",
		chipKey: "user",
		pillLabel: "shared with owner",
	},
};

const OWNER_NAMES = Array.from({ length: 12 }, (_, index) => `user-${index}`);

// More owners than the flyout search threshold. `zed` is only returned by a
// search, like a user beyond the first page of results.
const manyOwnersCategory: FilterCategory = {
	key: "owner",
	label: "Owner",
	getOptions: async (query) =>
		[...OWNER_NAMES, ...(query ? ["zed"] : [])]
			.filter((name) => name.includes(query))
			.map((name) => ({ label: name, value: name })),
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
	chipKeys: ["outdated", "dormant"],
	inlineOptions: true,
	inlineOptionsExclusive: true,
	inlineOptionsLabelOnly: true,
	inlineOptionsLabel: "Workspace is…",
	getOptions: async () => [
		{ label: "Outdated", value: "outdated", token: "outdated:true" },
		{ label: "Deletion pending", value: "dormant", token: "dormant:true" },
	],
};

function FilterComboboxHarness({
	categories,
	initialValue,
	onChange,
}: {
	categories: readonly FilterCategory[];
	initialValue: string;
	onChange: (value: string) => void;
}) {
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

const setup = (
	categories: readonly FilterCategory[],
	{
		initialValue = "",
		skipHover = false,
	}: { initialValue?: string; skipHover?: boolean } = {},
) => {
	// user-event moves the pointer between elements without a related target,
	// which reads as leaving the whole menu. Tests that click into a hover
	// flyout skip those synthetic hover events.
	const user = userEvent.setup({ skipHover });
	const onChange = vi.fn();
	render(
		<FilterComboboxHarness
			categories={categories}
			initialValue={initialValue}
			onChange={onChange}
		/>,
	);
	return {
		user,
		onChange,
		input: screen.getByRole("combobox", { name: "Search and filter" }),
		filtersButton: screen.getByRole("button", { name: "Toggle filters" }),
	};
};

describe("FilterCombobox", () => {
	it("opens from the Filters button with keyboard focus and navigates categories", async () => {
		const { user, onChange, input, filtersButton } = setup([ownerCategory]);

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
		const { user, onChange, input } = setup([
			{
				...ownerCategory,
				getOptions: async (query) =>
					query === "missing" ? [] : ownerCategory.getOptions(query),
			},
		]);

		await user.click(input);
		await user.type(input, "missing");

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("missing"));
	});

	it("replaces the selected Workspace attribute", async () => {
		const { user, onChange, filtersButton } = setup([attributesCategory]);

		await user.click(filtersButton);
		await user.click(await screen.findByRole("option", { name: "Outdated" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("outdated:true"),
		);
		await user.click(
			await screen.findByRole("option", { name: "Deletion pending" }),
		);

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("dormant:true"),
		);
	});

	it("removes a selected inline option", async () => {
		const { user, onChange, filtersButton } = setup([statusCategory], {
			initialValue: "status:running",
		});

		await user.click(filtersButton);
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
	});

	it("keeps the main menu usable and focused after selecting a flyout option", async () => {
		const { user, onChange, input, filtersButton } = setup(
			[ownerCategory, statusCategory],
			{ skipHover: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
		expect(input).toHaveFocus();
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice status:running"),
		);
	});

	it("selects an inline option by keyboard after hovering a category", async () => {
		const { user, onChange, filtersButton } = setup([
			ownerCategory,
			statusCategory,
		]);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await screen.findByRole("button", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
		);
	});

	it("keeps the category row highlighted after leaving it with ArrowLeft", async () => {
		const { user, onChange, filtersButton } = setup([
			ownerCategory,
			statusCategory,
		]);

		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{ArrowLeft}");
		await waitFor(() =>
			expect(
				screen.queryByRole("option", { name: "alice" }),
			).not.toBeInTheDocument(),
		);
		await user.keyboard("{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("keeps search text typed before an inline category prefix", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "dev status:");
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running dev"),
		);
		expect(onChange).not.toHaveBeenCalledWith("dev status:");
		expect(input).toHaveValue("dev");
	});

	it("commits a typed inline value that is not a suggestion with Enter", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "status:starting");

		// Enter waits for the debounced suggestions, so retry until it commits.
		await waitFor(async () => {
			await user.keyboard("{Enter}");
			expect(onChange).toHaveBeenLastCalledWith("status:starting");
		});
	});

	it("opens a category narrowed by typed text when clicked", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "ow");
		await user.click(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("option", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("searches the hover flyout through the category loader", async () => {
		const { user, onChange, filtersButton } = setup([manyOwnersCategory], {
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.type(search, "nobody");
		expect(search).toHaveFocus();

		await user.clear(search);
		await user.type(search, "zed");
		await user.click(await screen.findByRole("button", { name: "zed" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("owner:zed"));
	});

	it("keeps the category search field while results shrink", async () => {
		const { user, filtersButton } = setup([manyOwnersCategory]);

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.click(search);
		await user.type(search, "user-1");
		await waitFor(() =>
			expect(screen.queryByRole("option", { name: "user-2" })).toBeNull(),
		);

		expect(search).toHaveFocus();
	});

	it("commits options under the scope toggle key by default", async () => {
		const { user, onChange, filtersButton } = setup([scopedOwnerCategory]);

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		await user.click(await screen.findByRole("option", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("user:alice"),
		);
	});

	it("rewrites the applied chip when the scope toggle is switched off", async () => {
		const { user, onChange, filtersButton } = setup([scopedOwnerCategory], {
			initialValue: "user:alice",
		});

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		await user.click(
			await screen.findByRole("switch", {
				name: "Include workspaces shared with alice",
			}),
		);

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("removing the scope pill narrows the applied chip", async () => {
		const { user, onChange } = setup([scopedOwnerCategory], {
			initialValue: "user:alice",
		});

		await user.click(
			screen.getByRole("button", { name: "Remove shared with owner" }),
		);

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("removes the scope pill before its chip with Backspace", async () => {
		const { user, onChange, input } = setup([scopedOwnerCategory], {
			initialValue: "user:alice",
		});

		await user.click(input);
		await user.keyboard("{Backspace}");
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);

		await user.keyboard("{Backspace}");
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
	});

	it("opens the Owner flyout with its toggle when typing shared", async () => {
		const { user, onChange, input } = setup([scopedOwnerCategory], {
			initialValue: "owner:alice",
			skipHover: true,
		});

		await user.click(input);
		await user.type(input, "shared");
		await user.click(
			await screen.findByRole("switch", {
				name: "Include workspaces shared with alice",
			}),
		);

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("user:alice"),
		);
		expect(input).toHaveValue("");
	});

	it("clears the typed text when picking an owner from the scope flyout", async () => {
		const { user, onChange, input } = setup([scopedOwnerCategory], {
			skipHover: true,
		});

		await user.click(input);
		await user.type(input, "sha");
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("user:alice"),
		);
		expect(input).toHaveValue("");
	});
});
