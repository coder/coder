import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { render } from "#/testHelpers/renderHelpers";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterCategory } from "./types";

const ownerCategory: FilterCategory = {
	key: "owner",
	label: "Owner",
	showWhenSingleOption: true,
	getOptions: async () => [{ label: "alice", value: "alice" }],
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
	getOptions: async (query) =>
		[{ label: "Running", value: "running" }].filter((option) =>
			option.value.includes(query),
		),
};

const emptyTemplateCategory: FilterCategory = {
	key: "template",
	label: "Template",
	getOptions: async () => [],
};

const neverResolves = () => new Promise<never>(() => {});

const attributesCategory: FilterCategory = {
	key: "attribute",
	label: "Attributes",
	chipKeys: ["outdated", "dormant"],
	inlineOptions: true,
	inlineOptionsExclusive: true,
	chipLabelOnly: true,
	inlineOptionsLabel: "Workspace is…",
	getOptions: async () => [
		{ label: "Outdated", value: "outdated", token: "outdated:true" },
		{ label: "Dormant", value: "dormant", token: "dormant:true" },
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
		filtersButton: screen.getByRole("button", { name: "Filters" }),
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
		await user.click(await screen.findByRole("option", { name: "Dormant" }));

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
		await user.keyboard("{ArrowDown}{ArrowLeft}{ArrowRight}");
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
		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent("No filters found"),
		);
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("status:starting");
	});

	it("commits a typed inline value with Enter before suggestions load", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "status:starting{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("status:starting");
	});

	it("selects the first matching inline option with Enter before suggestions load", async () => {
		const { user, onChange, input, filtersButton } = setup([
			ownerCategory,
			statusCategory,
		]);

		// Load the unfiltered Status options first.
		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Running" });
		await user.click(input);
		await user.type(input, "status:run{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("status:running");
	});

	it("keeps search text typed before an inline prefix when completing with Tab", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "foo status:ru");
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{ArrowDown}{Tab}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running foo"),
		);
	});

	it("applies typed text that could be a filter only when the menu is dismissed", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "run");
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
		expect(onChange).not.toHaveBeenCalledWith("run");

		await user.keyboard("{Escape}");
		expect(onChange).toHaveBeenLastCalledWith("run");
	});

	it("applies typed text that could be a filter when switching to the full filter list", async () => {
		const { user, onChange, input, filtersButton } = setup([
			ownerCategory,
			statusCategory,
		]);

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "ali");
		await user.click(filtersButton);

		expect(onChange).toHaveBeenLastCalledWith("ali");
	});

	it("keeps every applied Workspace attribute when another filter changes", async () => {
		const { user, onChange, filtersButton } = setup(
			[statusCategory, attributesCategory],
			{ initialValue: "outdated:true dormant:true" },
		);

		await user.click(filtersButton);
		await user.click(await screen.findByRole("option", { name: "Running" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith(
				"outdated:true dormant:true status:running",
			),
		);
	});

	it("does not commit a chip token until it is finished", async () => {
		const { user, onChange, input } = setup([attributesCategory]);

		await user.click(input);
		await user.type(input, "outdated:t");
		expect(onChange).toHaveBeenLastCalledWith("");
		await user.type(input, "rue ");

		expect(onChange).toHaveBeenLastCalledWith("outdated:true");
		expect(input).toHaveValue("");
	});

	it("announces loading suggestions while typing", async () => {
		const getOptions = vi.fn(neverResolves);
		const { user, input } = setup([{ ...ownerCategory, getOptions }]);

		await user.click(input);
		await user.type(input, "ali");

		expect(screen.getByRole("status")).toHaveTextContent("Loading suggestions");
		await waitFor(() => expect(getOptions).toHaveBeenCalledWith("ali"));
		expect(screen.getByRole("status")).toHaveTextContent("Loading suggestions");
	});

	it("announces loading instead of a previous suggestion error while typing", async () => {
		const { user, input } = setup([
			{
				...ownerCategory,
				getOptions: async (query) => {
					if (query) {
						throw new Error("failed");
					}
					return [];
				},
			},
		]);

		await user.click(input);
		await user.type(input, "zzz");
		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				"Couldn’t load suggestions.",
			),
		);
		await user.type(input, "y");

		expect(screen.getByRole("status")).toHaveTextContent("Loading suggestions");
	});

	it("keeps matching rows selectable while typed text is debounced", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "status:ru");
		await user.click(screen.getByRole("option", { name: "Running" }));

		expect(onChange).toHaveBeenLastCalledWith("status:running");
	});

	it("keeps an applied option when Enter completes typed text that located it", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			initialValue: "owner:alice status:running",
		});

		await user.click(input);
		await user.type(input, "ali");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{Enter}");
		expect(onChange).toHaveBeenLastCalledWith("owner:alice status:running");
		expect(input).toHaveValue("");

		await user.click(input);
		await user.type(input, "status:run");
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{Enter}");
		expect(onChange).toHaveBeenLastCalledWith("owner:alice status:running");
	});

	it("does not suggest values for text typed before opening all filters", async () => {
		const { user, onChange, input, filtersButton } = setup([
			{
				...ownerCategory,
				getOptions: async () => [
					{ label: "alice", value: "alice" },
					{ label: "runner", value: "runner" },
				],
			},
		]);

		await user.click(input);
		await screen.findByRole("option", { name: "Owner" });
		await user.type(input, "run");
		await user.click(filtersButton);
		await user.keyboard("{End}{Enter}");

		expect(onChange).not.toHaveBeenCalledWith("owner:runner");
		expect(onChange).toHaveBeenLastCalledWith("run");
	});

	it("announces no filters only after suggestions load", async () => {
		const { user, input } = setup([emptyTemplateCategory]);

		await user.click(input);
		await user.type(input, "zzz");

		expect(screen.getByRole("status")).not.toHaveTextContent(
			"No filters found",
		);
		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent("No filters found"),
		);
	});

	it("announces an empty category", async () => {
		const { user, input } = setup([emptyTemplateCategory]);

		await user.click(input);
		await user.type(input, "template:");

		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				"No Template matches",
			),
		);
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

	it("retries a failed hover flyout search", async () => {
		let failed = false;
		const getOptions = vi.fn(async (query: string) => {
			if (query === "zed" && !failed) {
				failed = true;
				throw new Error("boom");
			}
			return manyOwnersCategory.getOptions(query);
		});
		const { user, onChange, filtersButton } = setup(
			[{ ...manyOwnersCategory, getOptions }],
			{ skipHover: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.type(search, "zed");
		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await user.click(await screen.findByRole("button", { name: "zed" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("owner:zed"));
	});

	it("retries a hover flyout whose options failed to load", async () => {
		let failed = false;
		const getOptions = vi.fn(async (query: string) => {
			if (!failed) {
				failed = true;
				throw new Error("boom");
			}
			return ownerCategory.getOptions(query);
		});
		const { user, onChange, filtersButton } = setup(
			[{ ...ownerCategory, getOptions }],
			{ skipHover: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await screen.findByText("Couldn’t load Owner options.");
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("removes a checked hover flyout option when clicked", async () => {
		const { user, onChange, filtersButton } = setup([ownerCategory], {
			initialValue: "owner:alice",
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
	});

	it("picks a category option with the keyboard from its search field", async () => {
		const { user, onChange, input, filtersButton } = setup([
			manyOwnersCategory,
		]);

		await user.click(filtersButton);
		await user.keyboard("{ArrowDown}{ArrowRight}");
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await screen.findByRole("option", { name: "user-0" });
		await user.click(search);
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:user-0"),
		);
		expect(input).toHaveFocus();
	});

	it("keeps the category search field while results shrink", async () => {
		const getOptions = vi.fn(manyOwnersCategory.getOptions);
		const { user, filtersButton } = setup([
			{ ...manyOwnersCategory, getOptions },
		]);

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.click(search);
		await user.type(search, "user-1");
		await waitFor(() => expect(getOptions).toHaveBeenCalledWith("user-1"));
		await getOptions.mock.results.at(-1)?.value;

		await waitFor(() => expect(search).toHaveFocus());
	});

	describe("on a mobile viewport", () => {
		const originalMatchMedia = window.matchMedia;

		beforeEach(() => {
			vi.stubGlobal("matchMedia", (query: string) => {
				const result = originalMatchMedia(query);
				return query === mobileViewportMediaQuery
					? { ...result, matches: true }
					: result;
			});
		});

		afterEach(() => {
			vi.unstubAllGlobals();
		});

		it("drills into a category and selects an option", async () => {
			const { user, onChange, filtersButton } = setup([
				ownerCategory,
				statusCategory,
			]);

			await user.click(filtersButton);
			await user.click(await screen.findByRole("option", { name: "Owner" }));
			await user.click(await screen.findByRole("option", { name: "alice" }));

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
			);
		});
	});
});
