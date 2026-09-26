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
		shouldAdvanceTime = true,
	}: {
		initialValue?: string;
		skipHover?: boolean;
		fakeTimers?: boolean;
		/** Lets real time move the fake clock too. */
		shouldAdvanceTime?: boolean;
	} = {},
) => {
	if (fakeTimers) {
		vi.useFakeTimers({ shouldAdvanceTime });
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

	const expectStatus = (text: string) =>
		waitFor(() => expect(screen.getByRole("status")).toHaveTextContent(text));

	const expectHighlighted = (name: string) =>
		expect(screen.getByRole("option", { name })).toHaveAttribute(
			"aria-selected",
			"true",
		);

	it("fetches hideable categories before the menu opens and skips others", async () => {
		const hideable = vi.fn(async () => [{ label: "Docker", value: "docker" }]);
		const other = vi.fn(async () => [{ label: "alice", value: "alice" }]);
		render(
			<FilterComboboxHarness
				categories={[
					{
						key: "template",
						label: "Template",
						hideWhenSingleOption: true,
						getOptions: hideable,
					},
					{ key: "owner", label: "Owner", getOptions: other },
				]}
				initialValue=""
				onChange={vi.fn()}
			/>,
		);
		await waitFor(() => expect(hideable).toHaveBeenCalledWith(""));
		expect(other).not.toHaveBeenCalled();
	});

	it("announces loading until hideable categories load, then offers them", async () => {
		const templates = heldSearch(
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => [],
			},
			"",
		);
		const { user, onChange, filtersButton } = setup([
			templates.category,
			ownerCategory,
		]);

		await user.click(filtersButton);
		expect(screen.getByRole("status")).toHaveTextContent("Loading filters");
		await act(async () =>
			templates.resolve([
				{ label: "docker", value: "docker" },
				{ label: "k8s", value: "k8s" },
			]),
		);
		await screen.findByRole("option", { name: "Template" });
		await user.keyboard("{Home}{ArrowRight}");
		await user.click(await screen.findByRole("option", { name: "k8s" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("template:k8s"),
		);
	});

	it("highlights the first category once placeholder rows are replaced", async () => {
		const templates = heldSearch(
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => [],
			},
			"",
		);
		const { user, onChange, filtersButton } = setup([
			ownerCategory,
			templates.category,
			statusCategory,
		]);

		await user.click(filtersButton);
		await screen.findByRole("option", { name: /Running/ });
		await act(async () =>
			templates.resolve([
				{ label: "docker", value: "docker" },
				{ label: "k8s", value: "k8s" },
			]),
		);
		await screen.findByRole("option", { name: "Template" });
		await user.keyboard("{Enter}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("keeps a highlight moved to an inline row while placeholder rows show", async () => {
		const templates = heldSearch(
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => [],
			},
			"",
		);
		const { user, onChange, filtersButton } = setup([
			ownerCategory,
			templates.category,
			statusCategory,
		]);

		await user.click(filtersButton);
		await screen.findByRole("option", { name: /Running/ });
		await user.keyboard("{ArrowDown}");
		await act(async () =>
			templates.resolve([
				{ label: "docker", value: "docker" },
				{ label: "k8s", value: "k8s" },
			]),
		);
		await screen.findByRole("option", { name: "Template" });
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
		);
	});

	it("matches typed text to a hideable category while its options load", async () => {
		const templates = heldSearch(
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => [],
			},
			"",
		);
		const { user, onChange, input } = setup([
			ownerCategory,
			templates.category,
		]);

		await user.click(input);
		await user.type(input, "tem");
		await user.keyboard("{ArrowDown}{Enter}");
		await act(async () =>
			templates.resolve([
				{ label: "docker", value: "docker" },
				{ label: "k8s", value: "k8s" },
			]),
		);
		await user.click(await screen.findByRole("option", { name: "k8s" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("template:k8s"),
		);
	});

	it("announces the error of a hideable category whose options failed to load", async () => {
		const { user, filtersButton } = setup([
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => {
					throw new Error("failed");
				},
			},
			ownerCategory,
		]);

		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Template" });
		await user.keyboard("{Home}{ArrowRight}");

		await waitFor(() =>
			expect(screen.getByRole("status")).toHaveTextContent(
				"Couldn’t load Template options.",
			),
		);
	});

	it("keeps the category list through a hideable category's Retry", async () => {
		let failed = false;
		const retry = Promise.withResolvers<FilterOption[]>();
		const { user, onChange, filtersButton } = setup(
			[
				ownerCategory,
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions: async () => {
						if (!failed) {
							failed = true;
							throw new Error("failed");
						}
						return retry.promise;
					},
				},
			],
			{ skipHover: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Template" }));
		await user.click(await screen.findByRole("button", { name: "Retry" }));
		expect(screen.getByRole("status")).not.toHaveTextContent("Loading filters");
		await user.hover(screen.getByRole("option", { name: "Template" }));
		await act(async () =>
			retry.resolve([
				{ label: "docker", value: "docker" },
				{ label: "k8s", value: "k8s" },
			]),
		);
		await user.click(await screen.findByRole("button", { name: "k8s" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("template:k8s"),
		);
	});

	it("ignores hidden options for suggestions but keeps hidden categories reachable by name and prefix", async () => {
		const getOptions = vi.fn(async (query: string) =>
			[{ label: "Docker", value: "docker" }].filter((option) =>
				option.label.toLowerCase().includes(query.toLowerCase()),
			),
		);
		const { user, input, onChange } = setup(
			[
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions,
				},
			],
			{ fakeTimers: true },
		);
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		expect(onChange).toHaveBeenLastCalledWith("dock");
		expect(getOptions).not.toHaveBeenCalledWith("dock");

		await user.clear(input);
		await user.type(input, "templ");
		await settleTypedText();
		expect(onChange).not.toHaveBeenCalledWith("templ");

		await user.clear(input);
		await user.type(input, "template:");
		await user.click(await screen.findByRole("option", { name: "Docker" }));
		expect(onChange).toHaveBeenLastCalledWith("template:docker");
	});

	const docker = { label: "Docker", value: "docker" };

	const setupDeferredTemplateLoads = (
		initialValue: string,
		{
			category = {},
			getFilteredOptions = () => Promise.resolve([docker]),
			skipHover = false,
			// A clock that follows real time lets a slow run fire the lookup timeout
			// before a deferred load settles.
			shouldAdvanceTime = false,
		}: {
			category?: Partial<FilterCategory>;
			getFilteredOptions?: (query: string) => Promise<FilterOption[]>;
			skipHover?: boolean;
			shouldAdvanceTime?: boolean;
		} = {},
	) => {
		const [firstLoad, retryLoad] = [
			Promise.withResolvers<FilterOption[]>(),
			Promise.withResolvers<FilterOption[]>(),
		];
		let unfilteredLoads = 0;
		const rendered = setup(
			[
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions: (query) => {
						if (query !== "") {
							return getFilteredOptions(query);
						}
						unfilteredLoads += 1;
						return unfilteredLoads === 1
							? firstLoad.promise
							: retryLoad.promise;
					},
					...category,
				},
			],
			{ initialValue, fakeTimers: true, skipHover, shouldAdvanceTime },
		);
		return { ...rendered, firstLoad, retryLoad };
	};

	it("searches text matching a hideable category whose first load settles at one option", async () => {
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads("");
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(async () => firstLoad.resolve([docker]));

		expect(onChange).toHaveBeenLastCalledWith("dock");
	});

	it("holds back text matching a hideable category whose first load settles at one option with its chip", async () => {
		const { user, input, onChange, firstLoad } =
			setupDeferredTemplateLoads("template:docker");
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(async () => firstLoad.resolve([docker]));
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("template:docker dock");
	});

	it("searches text matching a hideable category whose chip is removed before its first load settles", async () => {
		const { user, input, onChange, firstLoad } =
			setupDeferredTemplateLoads("template:docker");
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await user.click(
			screen.getByRole("button", { name: "Remove template:docker" }),
		);
		await act(async () => firstLoad.resolve([docker]));

		expect(onChange).toHaveBeenLastCalledWith("dock");
	});

	it("holds back text matching a hideable category when its first load and lookup each fit the timeout but their sum does not", async () => {
		const loadMs = TYPED_TEXT_LOOKUP_TIMEOUT_MS * 0.8;
		const lookupMs = TYPED_TEXT_LOOKUP_TIMEOUT_MS * 0.4;
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads(
			"",
			{
				getFilteredOptions: () =>
					new Promise((resolve) => setTimeout(resolve, lookupMs, [docker])),
			},
		);
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(() => vi.advanceTimersByTimeAsync(loadMs));
		await act(async () =>
			firstLoad.resolve([
				{ label: "Docker", value: "docker" },
				{ label: "Kubernetes", value: "kubernetes" },
			]),
		);
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("dock");
	});

	it("holds back text matching a hideable category whose first load fails", async () => {
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads("");
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(async () => firstLoad.reject(new Error("boom")));
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("dock");
	});

	it("holds back text matching a hideable category whose first load already failed", async () => {
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads("");
		await act(async () => firstLoad.reject(new Error("boom")));
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("dock");
	});

	it("searches text matching a hideable category whose Retry settles at one option", async () => {
		const { user, input, filtersButton, onChange, firstLoad, retryLoad } =
			setupDeferredTemplateLoads("", {
				skipHover: true,
				shouldAdvanceTime: true,
			});
		await act(async () => firstLoad.reject(new Error("boom")));
		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Template" }));
		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await user.click(input);
		// Only the test moves the clock from here, so a slow run cannot fire
		// the lookup timeout.
		vi.setTimerTickMode("manual");
		await user.type(input, "dock");
		await settleTypedText();
		await act(async () => retryLoad.resolve([docker]));

		expect(onChange).toHaveBeenLastCalledWith("dock");
	});

	it("searches text matching a settled hideable category whose chip is removed during the lookup", async () => {
		const kubernetes = Promise.withResolvers<FilterOption[]>();
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads(
			"template:docker",
			{ getFilteredOptions: () => kubernetes.promise },
		);
		await act(async () => firstLoad.resolve([docker]));
		await user.click(input);
		await user.type(input, "kub");
		await settleTypedText();
		await user.click(
			screen.getByRole("button", { name: "Remove template:docker" }),
		);
		await act(async () =>
			kubernetes.resolve([{ label: "Kubernetes", value: "kubernetes" }]),
		);

		expect(onChange).toHaveBeenLastCalledWith("kub");
	});

	it("holds back text matching an inline category that sets hideWhenSingleOption", async () => {
		const { user, input, onChange, firstLoad } = setupDeferredTemplateLoads(
			"",
			{
				category: { inlineOptions: true },
			},
		);
		await user.click(input);
		await user.type(input, "dock");
		await settleTypedText();
		await act(async () => firstLoad.resolve([docker]));
		await act(() => vi.advanceTimersByTimeAsync(TYPED_TEXT_LOOKUP_TIMEOUT_MS));

		expect(onChange).not.toHaveBeenCalledWith("dock");
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
		const { user, onChange, input, filtersButton } = setup([ownerCategory]);

		await user.click(input);
		await user.type(input, "ali");
		const option = await screen.findByRole("option", { name: "alice" });
		await waitFor(() =>
			expect(option).toHaveAttribute("aria-selected", "false"),
		);
		await user.keyboard("{Enter}");

		expect(onChange).toHaveBeenLastCalledWith("ali");
		expect(input).toHaveValue("ali");
		expect(filtersButton).toHaveAttribute("aria-expanded", "false");
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
		expectHighlighted("alice");
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
				"No Template options.",
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
		await expectStatus("No Owner matches");

		await user.clear(search);
		await user.type(search, "zed");
		await user.click(await screen.findByRole("button", { name: "zed" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("owner:zed"));
	});

	it("clears the hover flyout search when another flyout shows or the menu closes", async () => {
		const userCategory = { ...manyOwnersCategory, key: "user", label: "User" };
		const { user, filtersButton } = setup([manyOwnersCategory, userCategory], {
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(
			await screen.findByRole("textbox", { name: "Search Owner" }),
		);
		await user.paste("user-1");
		await user.hover(screen.getByRole("option", { name: "User" }));
		await screen.findByRole("textbox", { name: "Search User" });
		await user.hover(screen.getByRole("option", { name: "Owner" }));
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		expect(search).toHaveValue("");

		await user.click(search);
		await user.paste("user-1");
		await user.click(document.body);
		await waitFor(() =>
			expect(filtersButton).toHaveAttribute("aria-expanded", "false"),
		);
		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		expect(
			await screen.findByRole("textbox", { name: "Search Owner" }),
		).toHaveValue("");
	});

	it("does not search the next flyout's loader with the previous search", async () => {
		const getUserOptions = vi.fn(manyOwnersCategory.getOptions);
		const getOwnerOptions = vi.fn(manyOwnersCategory.getOptions);
		const { user, filtersButton } = setup(
			[
				{ ...manyOwnersCategory, getOptions: getOwnerOptions },
				{
					...manyOwnersCategory,
					key: "user",
					label: "User",
					getOptions: getUserOptions,
				},
			],
			{ skipHover: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.type(
			await screen.findByRole("textbox", { name: "Search Owner" }),
			"user-1",
		);
		await waitFor(() => expect(getOwnerOptions).toHaveBeenCalledWith("user-1"));
		await user.hover(screen.getByRole("option", { name: "User" }));
		// A stale search would run as soon as the User flyout shows, before this
		// one settles.
		await user.click(
			await screen.findByRole("textbox", { name: "Search User" }),
		);
		await user.paste("user-2");
		await waitFor(() => expect(getUserOptions).toHaveBeenCalledWith("user-2"));

		expect(getUserOptions).not.toHaveBeenCalledWith("user-1");
		expect(getOwnerOptions.mock.calls).toEqual([[""], ["user-1"]]);
	});

	it("keeps a settled hover flyout search when a space is typed", async () => {
		const { user, onChange, filtersButton } = setup([manyOwnersCategory], {
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.click(search);
		await user.paste("zed");
		await screen.findByRole("button", { name: "zed" });
		await user.keyboard(" ");

		expect(screen.getByRole("status")).not.toHaveTextContent(
			"No Owner matches",
		);
		await user.click(screen.getByRole("button", { name: "zed" }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("owner:zed"));
	});

	it("announces a hover flyout search as loading until its results arrive", async () => {
		const owners = heldSearch(manyOwnersCategory, "zed");
		const { user, onChange, filtersButton } = setup([owners.category], {
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(
			await screen.findByRole("textbox", { name: "Search Owner" }),
		);
		await user.paste("zed");
		await expectStatus("Loading Owner options.");
		await act(async () => owners.resolve([{ label: "zed", value: "zed" }]));
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
		await expectStatus("Couldn’t load Owner options.");
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await user.click(await screen.findByRole("button", { name: "zed" }));

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("owner:zed"));
	});

	it("returns keyboard navigation to a listed category after a one-option filter is removed", async () => {
		const { user, onChange, input } = setup([
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: async () => [{ label: "Docker", value: "docker" }],
			},
			{
				...manyOwnersCategory,
				key: "owner",
				getOptions: async () => [
					{ label: "alice", value: "alice" },
					{ label: "bob", value: "bob" },
				],
			},
		]);
		await user.click(input);
		await user.type(input, "template:");
		await user.click(await screen.findByRole("option", { name: "Docker" }));
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("template:docker"),
		);
		await user.keyboard("{Enter}");
		await screen.findByRole("option", { name: "Docker" });
		await user.click(await screen.findByRole("option", { name: "Docker" }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
		await user.keyboard("{Enter}");
		await screen.findByRole("option", { name: "alice" });
		await user.keyboard("{ArrowDown}{Enter}");
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("keeps a hover flyout that typed text only hid", async () => {
		const { user, input } = setup(
			[{ ...ownerCategory, getOptions: neverResolves }, statusCategory],
			{ skipHover: true },
		);

		await user.click(input);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await expectStatus("Loading Owner options.");
		await user.type(input, "own");
		await waitFor(() =>
			expect(screen.getByRole("status")).not.toHaveTextContent(
				"Loading Owner options.",
			),
		);
		await user.clear(input);

		await expectStatus("Loading Owner options.");
	});

	it("does not open another flyout after a Retry hides the open one", async () => {
		const retry = Promise.withResolvers<undefined>();
		let calls = 0;
		const { user, filtersButton } = setup(
			[
				{ ...ownerCategory, getOptions: neverResolves },
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions: async () => {
						calls += 1;
						if (calls === 1) {
							throw new Error("boom");
						}
						await retry.promise;
						return [{ label: "Docker", value: "docker" }];
					},
				},
			],
			{ skipHover: true, fakeTimers: true },
		);

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Template" }));
		await user.click(await screen.findByRole("button", { name: "Retry" }));
		await act(async () => retry.resolve(undefined));
		await act(() => vi.advanceTimersByTimeAsync(1000));

		expect(screen.getByRole("status")).not.toHaveTextContent(
			"Loading Owner options.",
		);
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
		await expectStatus("Couldn’t load Owner options.");
		await user.click(screen.getByRole("button", { name: "Retry" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	// With no categoriesBeforeStatus, Owner is the only row above Status, so
	// ArrowUp from Status highlights it.
	const setupFailedInlineStatus = (
		categoriesBeforeStatus: readonly FilterCategory[] = [],
	) => {
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
			...setup([
				ownerCategory,
				...categoriesBeforeStatus,
				{ ...statusCategory, getOptions },
			]),
			retry,
		};
	};

	it("announces a failed inline category and retries it from the keyboard", async () => {
		const { user, onChange, filtersButton, retry } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await expectStatus("Couldn’t load Status options.");
		await user.keyboard("{End}");
		expectHighlighted("Retry");
		await user.keyboard("{Enter}");
		await expectStatus("Loading Status options.");
		expectHighlighted("Loading Status options.");
		await user.keyboard("{Enter}");
		expect(screen.getByRole("status")).toHaveTextContent(
			"Loading Status options.",
		);

		await act(async () => retry.resolve(undefined));
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{Enter}");
		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
		);
	});

	it("moves on from the first loaded option after a Retry hands it the highlight", async () => {
		const { user, filtersButton, retry } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await expectStatus("Couldn’t load Status options.");
		await user.keyboard("{End}{Enter}");
		await act(async () => retry.resolve(undefined));
		await waitFor(() => expectHighlighted("Running"));
		await user.keyboard("{ArrowUp}");

		expectHighlighted("Owner");
	});

	it("keeps a highlight typed over a pending Retry when the load finishes", async () => {
		const { user, filtersButton, retry } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await expectStatus("Couldn’t load Status options.");
		await user.keyboard("{End}{Enter}");
		await expectStatus("Loading Status options.");
		await user.keyboard("i");
		await act(async () => retry.resolve(undefined));
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{ArrowUp}");

		expectHighlighted("alice");
	});

	it("highlights the first row when reopened after a Retry finished while closed", async () => {
		const { user, filtersButton, retry } = setupFailedInlineStatus();

		await user.click(filtersButton);
		await expectStatus("Couldn’t load Status options.");
		await user.keyboard("{End}{Enter}");
		await expectStatus("Loading Status options.");
		await user.keyboard("{Escape}");
		await waitFor(() =>
			expect(filtersButton).toHaveAttribute("aria-expanded", "false"),
		);
		await act(async () => retry.resolve(undefined));
		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Running" });

		expectHighlighted("Owner");
	});

	it("hands a Retry's highlight to the first loaded option while placeholder rows show", async () => {
		const { user, onChange, filtersButton, retry } = setupFailedInlineStatus([
			{
				key: "template",
				label: "Template",
				hideWhenSingleOption: true,
				getOptions: neverResolves,
			},
		]);

		await user.click(filtersButton);
		await screen.findByRole("option", { name: "Retry" });
		await user.keyboard("{End}{Enter}");
		await screen.findByRole("option", { name: "Loading Status options." });
		expect(screen.getByRole("status")).toHaveTextContent("Loading filters");
		await act(async () => retry.resolve(undefined));
		await screen.findByRole("option", { name: "Running" });
		await user.keyboard("{Enter}");

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("status:running"),
		);
	});

	it("announces every failed inline category", async () => {
		const failing = async () => {
			throw new Error("boom");
		};
		const { filtersButton, user } = setup([
			{ ...statusCategory, getOptions: failing },
			{ ...attributesCategory, getOptions: failing },
		]);

		await user.click(filtersButton);

		await expectStatus(
			"Couldn’t load Status options. Couldn’t load Attributes options.",
		);
	});

	it("shows and announces an inline category's pending first load", async () => {
		const { filtersButton, user } = setup([
			{ ...statusCategory, getOptions: () => new Promise(() => {}) },
		]);

		await user.click(filtersButton);

		await screen.findByRole("option", { name: "Loading Status options." });
		await expectStatus("Loading Status options.");
	});

	it("announces a pending hover flyout Retry", async () => {
		let calls = 0;
		const getOptions = async () => {
			calls += 1;
			if (calls === 1) {
				throw new Error("boom");
			}
			return new Promise<FilterOption[]>(() => {});
		};
		const { user, filtersButton } = setup([{ ...ownerCategory, getOptions }], {
			skipHover: true,
		});

		await user.click(filtersButton);
		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await expectStatus("Couldn’t load Owner options.");
		await user.click(screen.getByRole("button", { name: "Retry" }));

		await expectStatus("Loading Owner options.");
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

		await expectStatus("Loading Status options.");
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

	it("clears every chip and the search text with Clear all", async () => {
		const { user, onChange, input } = setup(
			[ownerCategory, statusCategory, attributesCategory],
			{
				initialValue: "owner:alice status:running outdated:true dev",
				fakeTimers: true,
				skipHover: true,
			},
		);

		await user.click(input);
		await user.type(input, " ali");
		await user.click(screen.getByRole("button", { name: "Clear all" }));
		await settleTypedText();

		expect(onChange).toHaveBeenLastCalledWith("");
		expect(onChange).not.toHaveBeenCalledWith("dev ali");
		expect(input).toHaveValue("");
		expect(input).toHaveFocus();

		await user.hover(await screen.findByRole("option", { name: "Owner" }));
		await user.click(await screen.findByRole("button", { name: "alice" }));
		expect(onChange).toHaveBeenLastCalledWith("owner:alice");
	});

	it("leaves focus in place when Clear all is clicked without focus", async () => {
		const { user, input } = setup(
			[ownerCategory, statusCategory, attributesCategory],
			{ initialValue: "owner:alice status:running outdated:true" },
		);

		await user.click(screen.getByRole("button", { name: "Clear all" }));

		expect(input).not.toHaveFocus();
	});

	it("reaches Clear all from the input with Escape and Tab", async () => {
		const { user, input } = setup(
			[ownerCategory, statusCategory, attributesCategory],
			{ initialValue: "owner:alice status:running outdated:true" },
		);

		await user.click(input);
		await user.keyboard("{Escape}");
		await user.tab();

		expect(screen.getByRole("button", { name: "Clear all" })).toHaveFocus();
	});

	it("clears every chip with Clear all while a category is open", async () => {
		const { user, onChange, input, filtersButton } = setup(
			[ownerCategory, statusCategory, attributesCategory],
			{ initialValue: "owner:alice status:running outdated:true" },
		);

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		await screen.findByRole("option", { name: "alice" });
		await user.click(screen.getByRole("button", { name: "Clear all" }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
		await user.type(input, "dev");
		await user.keyboard("{Enter}");

		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("dev"));
	});

	it("returns focus to the input when Clear all is clicked from a category search", async () => {
		const { user, onChange, input, filtersButton } = setup(
			[manyOwnersCategory, statusCategory, attributesCategory],
			{
				initialValue: "owner:alice status:running outdated:true",
				skipHover: true,
			},
		);

		await user.click(filtersButton);
		await user.keyboard("{ArrowRight}");
		const search = await screen.findByRole("textbox", { name: "Search Owner" });
		await user.click(search);
		await user.click(screen.getByRole("button", { name: "Clear all" }));

		expect(input).toHaveFocus();
		await user.keyboard("dev");
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("dev"));
	});

	it("highlights a category row after Clear all from a category its chip listed", async () => {
		const { user, onChange, input } = setup(
			[
				ownerCategory,
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions: async () => [{ label: "docker", value: "docker" }],
				},
				statusCategory,
			],
			{ initialValue: "template:docker owner:alice status:running" },
		);

		await user.click(input);
		await user.type(input, "template:");
		await screen.findByRole("option", { name: "docker" });
		await user.click(screen.getByRole("button", { name: "Clear all" }));
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
		await user.keyboard("{Enter}");
		await user.click(await screen.findByRole("option", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it("highlights a category row after Clear all by keyboard from a row its chip listed", async () => {
		const { user, onChange, input } = setup(
			[
				ownerCategory,
				{
					key: "template",
					label: "Template",
					hideWhenSingleOption: true,
					getOptions: async () => [{ label: "docker", value: "docker" }],
				},
				statusCategory,
			],
			{ initialValue: "template:docker owner:alice status:running" },
		);

		await user.click(input);
		await screen.findByRole("option", { name: "Template" });
		await user.keyboard("{ArrowDown}");
		await user.keyboard("{Escape}");
		await user.tab();
		await user.keyboard("{Enter}");
		await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
		await user.keyboard("{Enter}");
		await user.click(await screen.findByRole("option", { name: "alice" }));

		await waitFor(() =>
			expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
		);
	});

	it.each([
		["the remove button", "click"],
		["Backspace", "Backspace"],
	])(
		"highlights a category row after removing the chip that listed the highlighted row with %s",
		async (_, removal) => {
			const { user, onChange, input } = setup(
				[
					ownerCategory,
					{
						key: "template",
						label: "Template",
						hideWhenSingleOption: true,
						getOptions: async () => [{ label: "docker", value: "docker" }],
					},
				],
				{ initialValue: "template:docker" },
			);

			await user.click(input);
			await screen.findByRole("option", { name: "Template" });
			await user.keyboard("{ArrowDown}{Escape}");
			if (removal === "click") {
				await user.click(
					screen.getByRole("button", { name: "Remove template:docker" }),
				);
			} else {
				await user.keyboard("{Backspace}");
			}
			await user.click(input);
			await user.keyboard("{Enter}");
			await user.click(await screen.findByRole("option", { name: "alice" }));

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
			);
		},
	);

	it.each([
		["the input", "input"],
		["the Filters button", "Filters"],
	])(
		"highlights a category row after the caller clears the chip that listed the highlighted row and %s reopens the menu",
		async (_, opener) => {
			const user = userEvent.setup();
			const onChange = vi.fn();
			const Harness = () => {
				const [value, setValue] = useState("template:docker");
				return (
					<>
						<FilterCombobox
							value={value}
							onChange={(next) => {
								onChange(next);
								setValue(next);
							}}
							categories={[
								ownerCategory,
								{
									key: "template",
									label: "Template",
									hideWhenSingleOption: true,
									getOptions: async () => [
										{ label: "docker", value: "docker" },
									],
								},
							]}
							placeholder="Search and filter"
						/>
						{/* Keeps focus where it is, like a caller update from elsewhere. */}
						<button
							type="button"
							onMouseDown={(event) => event.preventDefault()}
							onClick={() => setValue("")}
						>
							Reset
						</button>
					</>
				);
			};
			render(<Harness />);
			const input = screen.getByRole("combobox", { name: "Search and filter" });

			await user.click(input);
			await screen.findByRole("option", { name: "Template" });
			await user.keyboard("{ArrowDown}{Escape}");
			await user.click(screen.getByRole("button", { name: "Reset" }));
			if (opener === "input") {
				act(() => input.blur());
				await user.click(input);
			} else {
				await user.click(screen.getByRole("button", { name: "Filters" }));
			}
			await user.keyboard("{Enter}");
			await user.click(await screen.findByRole("option", { name: "alice" }));

			await waitFor(() =>
				expect(onChange).toHaveBeenLastCalledWith("owner:alice"),
			);
		},
	);

	it("moves focus to the input when a focused Clear all is clicked", async () => {
		const { user, input } = setup(
			[ownerCategory, statusCategory, attributesCategory],
			{ initialValue: "owner:alice status:running outdated:true" },
		);

		const clearAll = screen.getByRole("button", { name: "Clear all" });
		clearAll.focus();
		await user.click(clearAll);

		expect(input).toHaveFocus();
	});

	it.each([
		["Enter", "{Enter}"],
		["Space", " "],
	])(
		"clears every chip with %s and moves focus to the input",
		async (_, key) => {
			const { user, onChange, input } = setup(
				[ownerCategory, statusCategory, attributesCategory],
				{ initialValue: "owner:alice status:running outdated:true dev" },
			);

			screen.getByRole("button", { name: "Clear all" }).focus();
			await user.keyboard(key);

			await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
			expect(input).toHaveFocus();
		},
	);

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

		it("does not announce a failed category it shows no flyout for", async () => {
			const { user, filtersButton } = setup(
				[
					{
						...ownerCategory,
						getOptions: async () => {
							throw new Error("boom");
						},
					},
					statusCategory,
				],
				{ skipHover: true },
			);

			await user.click(filtersButton);
			await user.hover(await screen.findByRole("option", { name: "Owner" }));
			await screen.findByRole("option", { name: "Running" });

			expect(screen.getByRole("status")).not.toHaveTextContent(
				"Couldn’t load Owner options.",
			);
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

		it("leaves focus off the input after Clear all from a category", async () => {
			const { user, onChange, input, filtersButton } = setup(
				[ownerCategory, statusCategory, attributesCategory],
				{ initialValue: "owner:alice status:running outdated:true" },
			);

			await user.click(filtersButton);
			await user.click(await screen.findByRole("option", { name: "Owner" }));
			await screen.findByRole("option", { name: "alice" });
			await user.click(screen.getByRole("button", { name: "Clear all" }));

			await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
			expect(input).not.toHaveFocus();
		});
	});
});
