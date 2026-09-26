import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { render } from "#/testHelpers/renderHelpers";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import { FilterCombobox, SEARCHABLE_OPTION_COUNT } from "./FilterCombobox";
import { SEARCH_DEBOUNCE_MS } from "./queries";
import type { FilterCategory, FilterOption } from "./types";
import {
	SUGGESTIONS_ERROR_MESSAGE,
	TYPED_TEXT_LOOKUP_TIMEOUT_MS,
} from "./useFilterCombobox";

const ownerCategory: FilterCategory = {
	key: "owner",
	label: "Owner",
	getOptions: async () => [{ label: "alice", value: "alice" }],
};

const OWNER_NAMES = Array.from(
	{ length: SEARCHABLE_OPTION_COUNT + 2 },
	(_, index) => `user-${index}`,
);

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

const heldSearch = (base: FilterCategory, query: string) => {
	const search = Promise.withResolvers<FilterOption[]>();
	const category: FilterCategory = {
		...base,
		getOptions: (text) =>
			text === query ? search.promise : base.getOptions(text),
	};
	return { category, resolve: search.resolve };
};

// Fires the debounce and runs callbacks of lookups that have already resolved.
const settleTypedText = () =>
	act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));

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
		fakeTimers = false,
	}: { initialValue?: string; skipHover?: boolean; fakeTimers?: boolean } = {},
) => {
	if (fakeTimers) {
		vi.useFakeTimers({ shouldAdvanceTime: true });
	}
	// user-event moves the pointer between elements without a related target,
	// which reads as leaving the whole menu. Tests that click into a hover
	// flyout skip those synthetic hover events.
	const user = userEvent.setup(
		fakeTimers
			? { skipHover, advanceTimers: vi.advanceTimersByTime }
			: { skipHover },
	);
	const onChange = vi.fn();
	const { unmount } = render(
		<FilterComboboxHarness
			categories={categories}
			initialValue={initialValue}
			onChange={onChange}
		/>,
	);
	return {
		user,
		onChange,
		unmount,
		input: screen.getByRole("combobox", { name: "Search and filter" }),
		filtersButton: screen.getByRole("button", { name: "Filters" }),
	};
};

describe("FilterCombobox", () => {
	afterEach(() => {
		vi.useRealTimers();
	});

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

	it("selects the highlighted inline option with Enter before suggestions load", async () => {
		const { user, onChange, input, filtersButton } = setup([
			ownerCategory,
			statusCategory,
		]);

		// Cached Status rows are filtered locally, so Running is highlighted
		// while the typed query is still debounced.
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
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			fakeTimers: true,
		});

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "run");
		await settleTypedText();
		expect(onChange).not.toHaveBeenCalledWith("run");

		await user.keyboard("{Escape}");
		expect(onChange).toHaveBeenLastCalledWith("run");
	});

	it("holds back typed text that matches an option beyond the first page", async () => {
		const owners = heldSearch(manyOwnersCategory, "zed");
		const { user, onChange, input } = setup([owners.category], {
			fakeTimers: true,
		});

		await user.click(input);
		await screen.findByRole("option", { name: "Owner" });
		await user.type(input, "zed");
		await act(async () => owners.resolve([{ label: "zed", value: "zed" }]));
		await settleTypedText();
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("zed");
	});

	it("holds back typed text that starts an inline category name", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			fakeTimers: true,
		});

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "sta");
		await settleTypedText();

		expect(onChange).not.toHaveBeenCalledWith("sta");
	});

	it("applies typed text as a search when its option lookup fails", async () => {
		const { user, onChange, input } = setup([
			{
				...ownerCategory,
				getOptions: async (query) => {
					if (query) {
						throw new Error("failed");
					}
					return ownerCategory.getOptions(query);
				},
			},
		]);

		await user.click(input);
		await user.type(input, "zzz");

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("zzz"));
	});

	it("applies typed text as a search when its option lookup does not settle", async () => {
		const { user, onChange, input } = setup(
			[heldSearch(ownerCategory, "zzz").category],
			{ fakeTimers: true },
		);

		await user.click(input);
		await user.type(input, "zzz");
		await settleTypedText();
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).toHaveBeenLastCalledWith("zzz");
	});

	it("drops a typed-text lookup that resolves after the text changed", async () => {
		const owner = heldSearch(ownerCategory, "foo");
		const { user, onChange, input } = setup([owner.category], {
			fakeTimers: true,
		});

		await user.click(input);
		await user.type(input, "foo");
		await settleTypedText();
		await user.clear(input);
		await user.type(input, "ali");
		await act(async () => owner.resolve([]));
		await settleTypedText();

		expect(onChange).not.toHaveBeenCalledWith("foo");
	});

	it("drops a typed-text lookup that resolves after unmounting", async () => {
		const owner = heldSearch(ownerCategory, "zzz");
		const { user, onChange, input, unmount } = setup([owner.category], {
			fakeTimers: true,
		});

		await user.click(input);
		await user.type(input, "zzz");
		await settleTypedText();
		unmount();
		await act(async () => owner.resolve([]));

		expect(onChange).not.toHaveBeenCalledWith("zzz");
	});

	it("applies unmatched typed text after a chip is removed during the debounce", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			initialValue: "owner:alice",
			fakeTimers: true,
		});

		await user.click(input);
		await user.type(input, "xyz");
		await user.click(
			screen.getByRole("button", { name: "Remove owner:alice" }),
		);
		await settleTypedText();

		expect(onChange).toHaveBeenLastCalledWith("xyz");
	});

	it("holds back typed text once one category's search matches while another is pending", async () => {
		const templateCategory: FilterCategory = {
			key: "template",
			label: "Template",
			getOptions: async (query) =>
				query === "zed" ? [{ label: "zed", value: "zed" }] : [],
		};
		const { user, onChange, input } = setup(
			[heldSearch(ownerCategory, "zed").category, templateCategory],
			{ fakeTimers: true },
		);

		await user.click(input);
		await user.type(input, "zed");
		await settleTypedText();
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("zed");
	});

	it("drops the deleted search when Backspace continues into a chip", async () => {
		const { user, onChange, input } = setup([ownerCategory], {
			initialValue: "owner:alice zzz",
			fakeTimers: true,
		});

		await user.click(input);
		await user.keyboard("{End}{Backspace}{Backspace}{Backspace}{Backspace}");
		await settleTypedText();

		expect(onChange).toHaveBeenLastCalledWith("");
	});

	it("drops a cleared search when a flyout option is picked during the debounce", async () => {
		const { user, onChange, input } = setup([ownerCategory], {
			initialValue: "zzz",
			skipHover: true,
			fakeTimers: true,
		});

		await user.click(input);
		await user.clear(input);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));
		await settleTypedText();

		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
		expect(onChange).not.toHaveBeenCalledWith("owner:alice zzz");
		expect(input).toHaveValue("");
	});

	it("keeps the applied search when a flyout option is picked from all filters", async () => {
		const { user, onChange, filtersButton } = setup([ownerCategory], {
			initialValue: "zzz",
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice zzz"),
		);
	});

	it("keeps the applied search when a typed drill-in value is committed", async () => {
		const { user, onChange, input, filtersButton } = setup(
			[
				{
					...ownerCategory,
					getOptions: async (query) =>
						query === "carol" ? [] : ownerCategory.getOptions(query),
				},
			],
			{ initialValue: "zzz" },
		);

		await user.click(filtersButton);
		await user.keyboard("{ArrowDown}{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.type(input, "carol");
		await waitFor(() =>
			expect(
				screen.queryByRole("option", { name: "alice" }),
			).not.toBeInTheDocument(),
		);
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:carol zzz"),
		);
	});

	it("applies unmatched typed text with the remaining chips after a chip is removed during its lookup", async () => {
		const owner = heldSearch(ownerCategory, "xyz");
		const { user, onChange, input } = setup([owner.category, statusCategory], {
			initialValue: "owner:alice status:running",
			fakeTimers: true,
		});

		await user.click(input);
		await user.type(input, "xyz");
		await settleTypedText();
		await user.click(
			screen.getByRole("button", { name: "Remove owner:alice" }),
		);
		await act(async () => owner.resolve([]));
		await settleTypedText();

		expect(onChange).toHaveBeenLastCalledWith("status:running xyz");
	});

	it("does not apply held-back typed text when a chip is removed", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			initialValue: "owner:alice",
		});

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "run");
		await user.click(
			screen.getByRole("button", { name: "Remove owner:alice" }),
		);

		expect(onChange).toHaveBeenLastCalledWith("");
		expect(onChange).not.toHaveBeenCalledWith("run");
	});

	it("cancels a pending typed search when the menu is dismissed", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			fakeTimers: true,
		});

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "run");
		await user.keyboard("{Escape}");
		await user.type(input, "x{Backspace}");
		await user.keyboard("{Escape}");
		await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));

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

	it.each(["{Enter}", "{Escape}"])(
		"applies a typed chip token once with %s",
		async (key) => {
			const { user, onChange, input } = setup([attributesCategory], {
				initialValue: "dormant:true",
			});

			await user.click(input);
			await user.type(input, "dormant:true");
			await user.keyboard(key);

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("dormant:true"),
			);
			expect(onChange).not.toHaveBeenCalledWith("dormant:true dormant:true");
		},
	);

	it.each(["{Enter}", "{Escape}"])(
		"applies a typed chip token as a chip and keeps the rest as text with %s",
		async (key) => {
			const { user, onChange, input } = setup([attributesCategory]);

			await user.click(input);
			await user.type(input, "ali outdated:true");
			await user.keyboard(key);

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("outdated:true ali"),
			);
			expect(input).toHaveValue("ali");
		},
	);

	it("does not send an applied typed chip token again with the next pick", async () => {
		const { user, onChange, input, filtersButton } = setup([
			statusCategory,
			attributesCategory,
		]);

		await user.click(input);
		await user.type(input, "outdated:true");
		await user.keyboard("{Enter}");
		await user.click(filtersButton);
		await user.click(await screen.findByRole("option", { name: /Running/ }));

		expect(onChange).toHaveBeenLastCalledWith("outdated:true status:running");
	});

	it("keeps a typed inline prefix in the input when Filters is clicked", async () => {
		const { user, input, filtersButton } = setup([
			statusCategory,
			attributesCategory,
		]);

		await user.click(input);
		await user.type(input, "ali status:ru");
		await user.click(filtersButton);

		expect(input).toHaveValue("ali status:ru");
	});

	it("keeps a typed chip token applied while value lags the sent query", async () => {
		const user = userEvent.setup();
		const onChange = vi.fn();
		render(
			<FilterCombobox
				value=""
				onChange={onChange}
				categories={[attributesCategory]}
				placeholder="Search and filter"
			/>,
		);
		const input = screen.getByRole("combobox", { name: "Search and filter" });

		await user.click(input);
		await user.type(input, "outdated:true");
		await user.keyboard("{Enter}");
		await user.click(screen.getByRole("button", { name: "Filters" }));

		expect(onChange).toHaveBeenLastCalledWith("outdated:true");
	});

	it("does not restore a removed typed chip token on Escape", async () => {
		const { user, onChange, input } = setup([attributesCategory]);

		await user.click(input);
		await user.type(input, "outdated:true");
		await user.keyboard("{Enter}");
		await user.click(screen.getByRole("button", { name: "Remove outdated" }));
		await user.click(input);
		await user.keyboard("{Escape}");

		expect(onChange).toHaveBeenLastCalledWith("");
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

	it("announces a failed suggestion query while another is still fetching", async () => {
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
			heldSearch(emptyTemplateCategory, "zzz").category,
		]);

		await user.click(input);
		await user.type(input, "zzz");

		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				SUGGESTIONS_ERROR_MESSAGE,
			),
		);
	});

	it("activates flyout option buttons and Retry with Enter", async () => {
		let failed = false;
		const getOptions = vi.fn(async (query: string) => {
			if (query === "" && !failed) {
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
		(await screen.findByRole("button", { name: "Retry" })).focus();
		await user.keyboard("{Enter}");
		(await screen.findByRole("button", { name: "alice" })).focus();
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("keeps matching rows selectable while typed text is debounced", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await screen.findByRole("option", { name: "Running" });
		await user.type(input, "status:ru");
		await user.click(screen.getByRole("option", { name: "Running" }));

		expect(onChange).toHaveBeenLastCalledWith("status:running");
	});

	it("keeps matching value suggestions selectable while typed text is debounced", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await screen.findByRole("option", { name: "Owner" });
		await user.type(input, "ali");
		await user.click(screen.getByRole("option", { name: "alice" }));

		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
	});

	it("does not keep locally matched rows for a category whose search failed", async () => {
		const { user, onChange, input } = setup([
			{
				...ownerCategory,
				getOptions: async (query) => {
					if (query) {
						throw new Error("failed");
					}
					return [{ label: "alice", value: "alice" }];
				},
			},
		]);

		await user.click(input);
		await screen.findByRole("option", { name: "Owner" });
		await user.type(input, "ali");
		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				"Couldn’t load suggestions.",
			),
		);
		await user.keyboard("{Enter}");

		expect(onChange).not.toHaveBeenCalledWith("owner:alice");
	});

	it("removes an applied inline option with Enter in the full filter list", async () => {
		const { user, onChange, filtersButton } = setup([statusCategory], {
			initialValue: "status:running zzz",
		});

		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("zzz");
	});

	it("searches workspaces with Enter when free-typed text matches a filter", async () => {
		const { user, onChange, input } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "ali");
		const option = await screen.findByRole("option", { name: "alice" });
		await waitFor(() =>
			expect(option).toHaveAttribute("aria-selected", "false"),
		);
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("ali");
		expect(input).toHaveValue("ali");
		expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
	});

	it("completes the first match for free-typed text with Tab", async () => {
		const { user, onChange, input } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "ali");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{Tab}");

		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
	});

	it.each([
		["ArrowDown", "{ArrowDown}"],
		["End", "{End}"],
		["Ctrl+N", "{Control>}n{/Control}"],
	])("picks a match for free-typed text after %s", async (_, keys) => {
		const { user, onChange, input } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "ali");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard(`${keys}{Enter}`);

		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
	});

	it("picks a hovered match for free-typed text with Enter", async () => {
		const { user, onChange, input } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "ali");
		await user.hover(await screen.findByRole("option", { name: "alice" }));
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
	});

	it("searches free-typed text with Enter after a pointer move outside the rows", async () => {
		const { user, onChange, input } = setup([manyOwnersCategory]);

		await user.click(input);
		await user.type(input, "zed");
		fireEvent.pointerMove(input);
		await screen.findByRole("option", { name: "zed" });
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("zed");
	});

	it.each([
		["another row remains", [{ label: "alan", value: "alan" }]],
		["no row remains", []],
	])(
		"searches free-typed text with Enter after the highlighted row unmounts and %s",
		async (_, searchResults) => {
			const { user, onChange, input } = setup(
				[
					{
						key: "owner",
						label: "Owner",
						getOptions: async (query) =>
							query === ""
								? [
										{ label: "alice", value: "alice" },
										{ label: "alan", value: "alan" },
									]
								: searchResults,
					},
				],
				{ fakeTimers: true },
			);

			await user.click(input);
			await user.type(input, "al");
			await screen.findByRole("option", { name: "alice" });
			await user.keyboard("{ArrowDown}");
			await settleTypedText();
			await user.keyboard("{Enter}");

			expect(onChange).toHaveBeenLastCalledWith("al");
		},
	);

	it("completes a category with Tab for free-typed text", async () => {
		const { user, onChange, input } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "own");
		await screen.findByRole("option", { name: "Owner" });
		await user.keyboard("{Tab}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{Enter}");

		expect(input).toHaveFocus();
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("completes the top row with Tab when a category and an inline option match", async () => {
		const { user, onChange, input } = setup([
			ownerCategory,
			{
				...statusCategory,
				getOptions: async () => [{ label: "Offline", value: "offline" }],
			},
		]);

		await user.click(input);
		await user.type(input, "o");
		await screen.findByRole("option", { name: /Offline/ });
		await user.keyboard("{Tab}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("searches free-typed text with Enter after a highlighted row mounts again", async () => {
		const failFirstSearch = Promise.withResolvers<undefined>();
		let searched = false;
		const { user, onChange, input } = setup([
			{
				...ownerCategory,
				getOptions: async (query) => {
					if (query === "ali" && !searched) {
						searched = true;
						await failFirstSearch.promise;
						throw new Error("failed");
					}
					return ownerCategory.getOptions(query);
				},
			},
		]);

		await user.click(input);
		await user.type(input, "ali");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}");
		expect(screen.getByRole("option", { name: "alice" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await act(async () => failFirstSearch.resolve(undefined));
		await user.click(await screen.findByRole("button", { name: /retry/i }));
		await screen.findByRole("option", { name: "alice" });
		await user.click(input);
		await user.keyboard("{Enter}");

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("ali"));
	});

	it("completes an inline option with Tab for free-typed text", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory]);

		await user.click(input);
		await user.type(input, "runn");
		await screen.findByRole("option", { name: /Running/ });
		await user.keyboard("{Tab}");

		expect(onChange).toHaveBeenLastCalledWith("status:running");
	});

	it("keeps the first match highlighted for text typed inside a category", async () => {
		const { user, onChange, input, filtersButton } = setup([ownerCategory]);

		await user.click(filtersButton);
		await user.keyboard("{ArrowDown}{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.type(input, "ali");
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("keeps the first row highlighted in the all-filters list", async () => {
		const { user, onChange, input, filtersButton } = setup([ownerCategory], {
			skipHover: true,
		});

		await user.click(input);
		await user.type(input, "ali");
		await user.keyboard("{Escape}");
		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Owner" });
		await user.keyboard("{Enter}");
		await user.click(await screen.findByRole("option", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice ali"),
		);
	});

	it("keeps an applied option when Enter completes typed text that located it", async () => {
		const { user, onChange, input } = setup([ownerCategory, statusCategory], {
			initialValue: "owner:alice status:running",
		});

		await user.click(input);
		await user.type(input, "ali");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");
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
		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				"Couldn’t load Owner options.",
			),
		);
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	// Owner comes first so a highlight that leaves the Status rows lands on it.
	const setupFailedInlineStatus = () => {
		const retry = Promise.withResolvers<undefined>();
		let calls = 0;
		const getOptions = async (query: string) => {
			calls += 1;
			if (calls === 1) {
				throw new Error("boom");
			}
			await retry.promise;
			return statusCategory.getOptions(query);
		};
		return {
			...setup([ownerCategory, { ...statusCategory, getOptions }]),
			retry,
		};
	};

	const expectStatus = (text: string) =>
		waitFor(() => expect(screen.getByRole("status")).toHaveTextContent(text));

	it("announces a failed inline category and retries it from the keyboard", async () => {
		const { user, onChange, filtersButton, retry } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await expectStatus("Couldn’t load Status options.");
		await user.keyboard("{End}");
		expect(screen.getByRole("option", { name: "Retry" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await user.keyboard("{Enter}");
		await expectStatus("Loading Status options");
		expect(
			screen.getByRole("option", { name: "Loading Status options" }),
		).toHaveAttribute("aria-selected", "true");
		await user.keyboard("{Enter}");
		expect(screen.getByRole("status")).toHaveTextContent(
			"Loading Status options",
		);

		await act(async () => retry.resolve(undefined));
		await user.click(await screen.findByRole("option", { name: "Running" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
		);
	});

	it("announces a failed inline category after its typed prefix", async () => {
		const { user, input } = setupFailedInlineStatus();

		await user.click(input);
		await user.type(input, "status:");

		await expectStatus("Couldn’t load Status options.");
	});

	it("announces a retrying inline category after its typed prefix", async () => {
		const { user, input, filtersButton } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await user.keyboard("{End}{Enter}");
		await user.type(input, "status:");

		await expectStatus("Loading Status options");
	});

	it("applies a typed row of an inline category whose unfiltered load failed", async () => {
		const { user, input, onChange } = setup([
			ownerCategory,
			{
				...statusCategory,
				getOptions: async (query) => {
					if (query === "") {
						throw new Error("boom");
					}
					return statusCategory.getOptions(query);
				},
			},
		]);

		await user.click(input);
		await user.type(input, "runn");
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
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
		await user.keyboard("{ArrowDown}{Control>}n{/Control}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:user-1"),
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
			const { user, onChange, input, filtersButton } = setup([
				ownerCategory,
				statusCategory,
			]);

			await user.click(filtersButton);
			await user.click(await screen.findByRole("option", { name: "Owner" }));
			await user.click(await screen.findByRole("option", { name: "alice" }));

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
			);
			expect(input).not.toHaveFocus();
		});
	});
});
