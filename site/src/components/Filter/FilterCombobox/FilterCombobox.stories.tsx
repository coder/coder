import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	CircleDotIcon,
	LayoutGridIcon,
	MoonIcon,
	RefreshCwOffIcon,
	SlidersHorizontalIcon,
	UserIcon,
} from "lucide-react";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import { setupMatchMedia } from "#/testHelpers/matchMedia";
import { mobileViewportMediaQuery } from "#/utils/mobile";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterCategory, FilterOption } from "./types";

const meta: Meta<typeof FilterCombobox> = {
	title: "components/Filter/FilterCombobox",
	component: FilterCombobox,
};

export default meta;
type Story = StoryObj<typeof FilterCombobox>;

const statusOptions: FilterOption[] = [
	{ label: "Running", value: "running" },
	{ label: "Stopped", value: "stopped" },
];

const ownerOptions: FilterOption[] = [
	{
		label: "alice",
		value: "alice",
		startIcon: <Avatar fallback="alice" size="sm" />,
	},
	{
		label: "bob",
		value: "bob",
		startIcon: <Avatar fallback="bob" size="sm" />,
	},
];

const templateOptions: FilterOption[] = [
	{ label: "docker", value: "docker" },
	{ label: "kubernetes", value: "kubernetes" },
];

const attributeOptions: FilterOption[] = [
	{
		label: "Outdated",
		value: "outdated",
		token: "outdated:true",
		startIcon: <RefreshCwOffIcon />,
	},
	{
		label: "Dormant",
		value: "dormant",
		token: "dormant:true",
		startIcon: <MoonIcon />,
	},
];

const filterOptions = (
	options: readonly FilterOption[],
	query: string,
): FilterOption[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [...options];
	}
	return options.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};

const categories: FilterCategory[] = [
	{
		key: "status",
		label: "Status",
		icon: <CircleDotIcon />,
		getOptions: async (query) => filterOptions(statusOptions, query),
	},
	{
		key: "template",
		label: "Template",
		icon: <LayoutGridIcon />,
		getOptions: async (query) => filterOptions(templateOptions, query),
	},
	{
		key: "owner",
		label: "Owner",
		aliases: ["user"],
		icon: <UserIcon />,
		getOptions: async (query) => filterOptions(ownerOptions, query),
	},
];

// Attributes groups boolean workspace filters; each option commits its own
// `key:true` chip, and the category owns those keys for parsing.
const categoriesWithAttributes: FilterCategory[] = [
	...categories.map((category) =>
		category.key === "status" ? { ...category, inlineOptions: true } : category,
	),
	{
		key: "attribute",
		aliases: ["attributes"],
		label: "Attributes",
		icon: <SlidersHorizontalIcon />,
		chipKeys: ["outdated", "dormant"],
		inlineOptions: true,
		inlineOptionsLabel: "Workspace is…",
		inlineOptionsExclusive: true,
		chipLabelOnly: true,
		getOptions: async (query) => filterOptions(attributeOptions, query),
	},
];

// Chips split key and value into separate spans, so match on the chip's text.
const chip = (token: string) => (_: string, element: Element | null) =>
	element?.getAttribute("data-slot") === "combobox-chip" &&
	element.textContent === token;

const FilterComboboxHarness = ({
	initialQuery = "owner:me",
	categories: categoriesProp = categories,
}: {
	initialQuery?: string;
	categories?: readonly FilterCategory[];
}) => {
	const [query, setQuery] = useState(initialQuery);

	return (
		<FilterCombobox
			value={query}
			onChange={setQuery}
			categories={categoriesProp}
			placeholder="Search and filter…"
			className="max-w-lg"
		/>
	);
};

export const Default: Story = {
	render: () => <FilterComboboxHarness />,
};

export const CompactMainMenu: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={categoriesWithAttributes}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("combobox", { name: "Search and filter…" }),
		);
	},
};

export const MobileCategoryNavigation: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	parameters: {
		viewport: { defaultViewport: "mobile1" },
		pixel: { matrix: { viewports: ["phone"] } },
	},
	beforeEach: () =>
		setupMatchMedia({
			[mobileViewportMediaQuery]: true,
		}).restore,
	play: async ({ canvasElement }) => {
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			within(canvasElement).getByRole("combobox", {
				name: "Search and filter…",
			}),
		);
		await userEvent.click(await body.findByRole("option", { name: "Owner" }));
		await body.findByRole("option", { name: "alice" });
	},
};

const searchOwnerFlyout = async (canvasElement: HTMLElement, text: string) => {
	const body = within(canvasElement.ownerDocument.body);
	await userEvent.click(
		within(canvasElement).getByRole("combobox", { name: "Search and filter…" }),
	);
	await userEvent.hover(await body.findByRole("option", { name: "Owner" }));
	await userEvent.type(
		await body.findByRole("textbox", { name: "Search Owner" }),
		text,
	);
};

export const SearchableHoverFlyout: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				{
					key: "owner",
					label: "Owner",
					icon: <UserIcon />,
					getOptions: async () =>
						Array.from({ length: 12 }, (_, index) => ({
							label: `user-${index + 1}`,
							value: `user-${index + 1}`,
						})),
				},
			]}
		/>
	),
	play: ({ canvasElement }) => searchOwnerFlyout(canvasElement, "user-12"),
};

// The search field stays in the flyout when nothing matches.
export const SearchableHoverFlyoutNoMatches: Story = {
	...SearchableHoverFlyout,
	play: ({ canvasElement }) => searchOwnerFlyout(canvasElement, "nobody"),
};

// Inside a category, the filter toggle returns to the category list instead of
// closing the menu.
export const ToggleLeavesCategory: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await waitFor(() =>
			expect(body.getByRole("option", { name: "Running" })).toBeVisible(),
		);
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await waitFor(() =>
			expect(body.getByRole("option", { name: /^Status/ })).toBeVisible(),
		);
		await expect(canvas.queryByText("status:")).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await waitFor(() =>
			expect(body.queryByRole("option")).not.toBeInTheDocument(),
		);
	},
};

export const ActiveFilterIcon: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByTestId("filter-active-icon")).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "Remove owner:me" }),
		);
		await waitFor(() =>
			expect(
				canvas.queryByTestId("filter-active-icon"),
			).not.toBeInTheDocument(),
		);
	},
};

export const WrappedChipsKeepIconsOnFirstRow: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery="owner:me status:running template:docker outdated:true dormant:true"
			categories={categoriesWithAttributes}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText(chip("owner:me"))).toBeVisible();
		await expect(canvas.getByText(chip("outdated"))).toBeVisible();
	},
};

// Backspace with an empty input removes the last committed chip.
export const BackspaceRemovesLastChip: Story = {
	render: () => (
		<FilterComboboxHarness initialQuery="owner:me status:running" />
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await expect(canvas.getByText(chip("status:running"))).toBeVisible();
		await expect(canvas.getByText(chip("owner:me"))).toBeVisible();
		await userEvent.click(input);
		await userEvent.keyboard("{Backspace}");
		await waitFor(() =>
			expect(
				canvas.queryByText(chip("status:running")),
			).not.toBeInTheDocument(),
		);
		await expect(canvas.getByText(chip("owner:me"))).toBeVisible();
	},
};

export const OpenFilterMenu: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		// Wait for the popup animation before asserting its options.
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Status/i })).toBeVisible(),
		);
		await expect(body.getByRole("option", { name: /Template/i })).toBeVisible();
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Owner/i })).toBeVisible(),
		);
		await expect(input).toHaveFocus();
	},
};

export const FocusShowsCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Owner/i })).toBeVisible(),
		);
		await expect(body.getByRole("option", { name: /Status/i })).toBeVisible();
		await expect(body.getByRole("option", { name: /Template/i })).toBeVisible();
	},
};

export const TypeFacetPrefix: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await expect(canvas.getByText("status:")).toBeVisible();
		await waitFor(() => expect(body.getByText("Running")).toBeVisible());
		await expect(body.getByRole("status")).toHaveTextContent(
			"Filtering by Status",
		);
	},
};

export const TypeRawCategoryValue: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:starting");
		await userEvent.keyboard("{Enter}");
	},
};

export const TypeaheadMatchingCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "ow");
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Owner/i })).toBeVisible(),
		);
		await expect(
			body.queryByRole("option", { name: /Status/i }),
		).not.toBeInTheDocument();
		await userEvent.keyboard("{ArrowDown}{Enter}");
		await expect(canvas.getByText("owner:")).toBeVisible();
		await waitFor(() => expect(body.getByText("alice")).toBeVisible());
	},
};

// Many matching suggestions scroll inside the capped menu.
export const LongTypeaheadResults: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={categoriesWithAttributes.map((category) =>
				category.key === "owner" || category.key === "template"
					? {
							...category,
							getOptions: async (query) =>
								filterOptions(
									Array.from({ length: 20 }, (_, index) => ({
										label: `${category.key}-alpha-${index + 1}`,
										value: `${category.key}-alpha-${index + 1}`,
									})),
									query,
								),
						}
					: category,
			)}
		/>
	),
	play: async ({ canvasElement }) => {
		const input = within(canvasElement).getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "a");
		await within(canvasElement.ownerDocument.body).findByRole("option", {
			name: "template-alpha-1",
		});
	},
};

// Typed text that matches no filter opens no dropdown.
export const NoFilterMatches: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				{
					key: "owner",
					label: "Owner",
					icon: <UserIcon />,
					getOptions: async (query) => filterOptions(ownerOptions, query),
				},
			]}
		/>
	),
	play: async ({ canvasElement }) => {
		const input = within(canvasElement).getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "missing");
	},
};

export const TabCompletesTopCategory: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "ow");
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Owner/i })).toBeVisible(),
		);
		await userEvent.keyboard("{Tab}");
		await expect(canvas.getByText("owner:")).toBeVisible();
		await waitFor(() => expect(body.getByText("alice")).toBeVisible());
	},
};

// Regression: Enter must commit the highlighted row, not the first option.
// Categories render as status, template, owner; cmdk highlights the first row
// (status) on open, so arrowing down to template and pressing Enter must open
// Template, not Status.
export const EnterCommitsHighlightedCategory: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Template/i })).toBeVisible(),
		);
		await userEvent.keyboard("{ArrowDown}");
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Template/i })).toHaveAttribute(
				"aria-selected",
				"true",
			),
		);
		await userEvent.keyboard("{Enter}");
		await expect(canvas.getByText("template:")).toBeVisible();
		await expect(canvas.queryByText("status:")).not.toBeInTheDocument();
	},
};

// Regression: chips must render in the order they were added, not in the
// configured category order. Categories are status, template, owner; starting
// from owner:me and adding template then status must keep the visible order
// owner:me, template:docker, status:running.
export const PreservesChipInsertionOrder: Story = {
	render: () => <FilterComboboxHarness initialQuery="owner:me" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});

		const chipTokens = () =>
			canvas
				.getAllByRole("button", { name: /^Remove / })
				.map((button) =>
					(button.getAttribute("aria-label") ?? "").replace(/^Remove /, ""),
				);

		await userEvent.click(input);
		await userEvent.type(input, "template:");
		await waitFor(() => expect(body.getByText("docker")).toBeVisible());
		await userEvent.click(body.getByRole("option", { name: /docker/i }));
		await waitFor(() =>
			expect(canvas.getByText(chip("template:docker"))).toBeVisible(),
		);

		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await waitFor(() => expect(body.getByText("Running")).toBeVisible());
		await userEvent.click(body.getByRole("option", { name: /Running/i }));
		await waitFor(() =>
			expect(canvas.getByText(chip("status:running"))).toBeVisible(),
		);

		await waitFor(() =>
			expect(chipTokens()).toEqual([
				"owner:me",
				"template:docker",
				"status:running",
			]),
		);
	},
};

export const CrossCategoryValueSuggestions: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				{
					key: "status",
					label: "Status",
					icon: <CircleDotIcon />,
					getOptions: async (query) => filterOptions(statusOptions, query),
				},
				{
					key: "owner",
					label: "Owner",
					aliases: ["user"],
					icon: <UserIcon />,
					getOptions: async (query) =>
						filterOptions(
							[
								{ label: "testuser01", value: "testuser01" },
								{ label: "alice", value: "alice" },
							],
							query,
						),
				},
			]}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "test");
		await waitFor(() => expect(body.getByText("Owner")).toBeVisible());
		await expect(
			body.getByRole("option", { name: /testuser01/i }),
		).toBeVisible();
		await expect(
			body.queryByRole("option", { name: /^Owner$/i }),
		).not.toBeInTheDocument();
		await userEvent.click(body.getByRole("option", { name: /testuser01/i }));
		await expect(canvas.getByText(chip("owner:testuser01"))).toBeVisible();
		await expect(input).toHaveValue("");
	},
};

// Typing an inline category prefix narrows the main panel to that category
// instead of opening a second panel beside it.
export const TypedInlinePrefix: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={categoriesWithAttributes}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await body.findByRole("option", { name: "Running" });
	},
};

export const ClearAll: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery="owner:me template:docker status:running outdated:true"
			categories={categoriesWithAttributes}
		/>
	),
};

// Two chips are below the Clear all threshold.
export const ClearAllBelowThreshold: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery="owner:me status:running"
			categories={categoriesWithAttributes}
		/>
	),
};

// Clear all from an open category returns the menu to the full filter list.
export const ClearAllFromCategory: Story = {
	...ClearAll,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(canvas.getByRole("button", { name: "Filters" }));
		await userEvent.keyboard("{ArrowRight}");
		await userEvent.click(canvas.getByRole("button", { name: "Clear all" }));
	},
};

const singleTemplateCategories: FilterCategory[] = [
	{
		key: "owner",
		label: "Owner",
		icon: <UserIcon />,
		getOptions: async (query) => filterOptions(ownerOptions, query),
	},
	{
		key: "template",
		label: "Template",
		icon: <LayoutGridIcon />,
		hideWhenSingleOption: true,
		getOptions: async (query) =>
			filterOptions(templateOptions.slice(0, 1), query),
	},
];

const openFilterMenu = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Filters" }),
	);
	await within(canvasElement.ownerDocument.body).findByRole("option", {
		name: "Owner",
	});
};

// Template has one option, so the menu omits it.
export const SingleOptionCategoryHidden: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={singleTemplateCategories}
		/>
	),
	play: ({ canvasElement }) => openFilterMenu(canvasElement),
};

// Placeholder rows hold the category list until Template's options load.
export const CategoryListLoading: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				singleTemplateCategories[0],
				{
					...singleTemplateCategories[1],
					getOptions: () => new Promise<FilterOption[]>(() => {}),
				},
			]}
		/>
	),
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
	},
};

// The template:docker chip keeps Template listed.
export const SingleOptionCategoryWithChip: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery="template:docker"
			categories={singleTemplateCategories}
		/>
	),
	play: ({ canvasElement }) => openFilterMenu(canvasElement),
};

// Escape closes the popup without clearing the committed chips.
export const DismissOnEscape: Story = {
	render: () => <FilterComboboxHarness initialQuery="owner:me" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Status/i })).toBeVisible(),
		);
		await userEvent.keyboard("{Escape}");
		await waitFor(() =>
			expect(
				body.queryByRole("option", { name: /Status/i }),
			).not.toBeInTheDocument(),
		);
		await expect(canvas.getByText(chip("owner:me"))).toBeVisible();
	},
};

// Pressing outside the input dismisses the popup while keeping the chips.
export const DismissOnOutsideClick: Story = {
	render: () => <FilterComboboxHarness initialQuery="owner:me" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Status/i })).toBeVisible(),
		);
		await userEvent.click(canvasElement.ownerDocument.body);
		await waitFor(() =>
			expect(
				body.queryByRole("option", { name: /Status/i }),
			).not.toBeInTheDocument(),
		);
		await expect(canvas.getByText(chip("owner:me"))).toBeVisible();
	},
};

export const CategoryOptionsErrorRetry: Story = {
	render: () => {
		// The unfiltered options and the category view share the empty-query
		// lookup, so both of their initial fetches fail; the Retry click is the
		// next call.
		let failuresLeft = 2;
		return (
			<FilterComboboxHarness
				initialQuery=""
				categories={[
					{
						key: "status",
						label: "Status",
						icon: <CircleDotIcon />,
						getOptions: async (query) => {
							if (failuresLeft > 0) {
								failuresLeft -= 1;
								throw new Error("boom");
							}
							return filterOptions(statusOptions, query);
						},
					},
				]}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await body.findByText(/Couldn.t load Status options/, {
			ignore: '[role="status"], script, style',
		});
		await userEvent.click(body.getByRole("button", { name: /retry/i }));
		await body.findByText("Running");
	},
};

export const TypeaheadErrorRetry: Story = {
	render: () => {
		let thrown = false;
		return (
			<FilterComboboxHarness
				initialQuery=""
				categories={[
					{
						key: "owner",
						label: "Owner",
						icon: <UserIcon />,
						getOptions: async (query) => {
							if (query === "alice" && !thrown) {
								thrown = true;
								throw new Error("boom");
							}
							return filterOptions([{ label: "alice", value: "alice" }], query);
						},
					},
				]}
			/>
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "alice");
		await body.findByText(/Couldn.t load suggestions/, {
			ignore: '[role="status"], script, style',
		});
		await userEvent.click(body.getByRole("button", { name: /retry/i }));
		await body.findByRole("option", { name: /alice/i });
	},
};

// A category list shows a spinner while its options load.
export const CategoryOptionsLoading: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				{
					key: "owner",
					label: "Owner",
					icon: <UserIcon />,
					getOptions: () => new Promise<FilterOption[]>(() => {}),
				},
			]}
		/>
	),
	play: async ({ canvasElement }) => {
		await userEvent.click(
			within(canvasElement).getByRole("button", { name: "Filters" }),
		);
		await within(canvasElement.ownerDocument.body).findByRole("option", {
			name: "Owner",
		});
		await userEvent.keyboard("{ArrowRight}");
	},
};

// A category whose options resolve empty announces a category-aware empty state
// rather than the browsing "No filters found." copy.
export const CategoryEmptyState: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={[
				{
					key: "template",
					label: "Template",
					icon: <LayoutGridIcon />,
					getOptions: async () => [],
				},
			]}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "template:");
	},
};

// Typing a full chip token (a chip-only key like `outdated`, completed with a
// space) promotes it to a chip and clears the input; a half-typed token stays
// in the input instead of committing early.
export const TypingChipTokenCommitsChip: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={categoriesWithAttributes}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "outdated:true ");
	},
};
