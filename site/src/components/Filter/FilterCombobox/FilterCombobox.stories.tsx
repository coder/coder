import type { Meta, StoryObj } from "@storybook/react-vite";
import {
	CircleDotIcon,
	LayoutGridIcon,
	MoonIcon,
	RefreshCwOffIcon,
	Share2Icon,
	SlidersHorizontalIcon,
	UserIcon,
} from "lucide-react";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterCategory, FilterOption, SearchResult } from "./types";

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
		startIcon: <Avatar fallback="alice" size="md" />,
	},
	{
		label: "bob",
		value: "bob",
		startIcon: <Avatar fallback="bob" size="md" />,
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
		startIcon: (
			<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
				<RefreshCwOffIcon className="size-icon-sm" />
			</span>
		),
	},
	{
		label: "Dormant",
		value: "dormant",
		token: "dormant:true",
		startIcon: (
			<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
				<MoonIcon className="size-icon-sm" />
			</span>
		),
	},
	{
		label: "Shared",
		value: "shared",
		token: "shared:true",
		startIcon: (
			<span className="flex size-[--avatar-default] shrink-0 items-center justify-center">
				<Share2Icon className="size-icon-sm" />
			</span>
		),
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
	...categories,
	{
		key: "attributes",
		label: "Attributes",
		icon: <SlidersHorizontalIcon />,
		chipKeys: ["outdated", "dormant", "shared"],
		getOptions: async (query) => filterOptions(attributeOptions, query),
	},
];

const FilterComboboxHarness = ({
	initialQuery = "owner:me",
	getSearchResults,
	onSearchResultSelect,
	searchResultsLabel,
	categories: categoriesProp = categories,
}: {
	initialQuery?: string;
	getSearchResults?: (query: string) => Promise<SearchResult[]>;
	onSearchResultSelect?: (result: SearchResult) => void;
	searchResultsLabel?: string;
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
			getSearchResults={getSearchResults}
			onSearchResultSelect={onSearchResultSelect}
			searchResultsLabel={searchResultsLabel}
		/>
	);
};

/** The popup renders under document.body; scope queries there to find it. */
const bodyOf = (canvasElement: HTMLElement) =>
	within(canvasElement.ownerDocument.body);

export const Default: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("combobox", { name: "Search and filter…" }),
		).toBeVisible();
		await expect(canvas.getByText("owner:me")).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Remove owner:me" }),
		).toBeVisible();
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
		await expect(canvas.getByText("status:running")).toBeVisible();
		await expect(canvas.getByText("owner:me")).toBeVisible();
		await userEvent.click(input);
		await userEvent.keyboard("{Backspace}");
		await waitFor(() =>
			expect(canvas.queryByText("status:running")).not.toBeInTheDocument(),
		);
		await expect(canvas.getByText("owner:me")).toBeVisible();
	},
};

export const OpenFilterMenu: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		// Popup is portaled and animates open; wait until options are visible.
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Status/i })).toBeVisible(),
		);
		await expect(body.getByRole("option", { name: /Template/i })).toBeVisible();
		await expect(body.getByRole("option", { name: /Owner/i })).toBeVisible();
		await expect(input).toHaveFocus();
	},
};

export const FocusShowsCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
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
		const body = bodyOf(canvasElement);
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

export const TypeaheadMatchingCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "ow");
		await expect(body.getByRole("option", { name: /Owner/i })).toBeVisible();
		await expect(
			body.queryByRole("option", { name: /Status/i }),
		).not.toBeInTheDocument();
		await userEvent.keyboard("{Enter}");
		await expect(canvas.getByText("owner:")).toBeVisible();
		await waitFor(() => expect(body.getByText("alice")).toBeVisible());
	},
};

export const TabCompletesTopCategory: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "ow");
		await expect(body.getByRole("option", { name: /Owner/i })).toBeVisible();
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
		const body = bodyOf(canvasElement);
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

export const LiveResourcePreviews: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			searchResultsLabel="Workspaces"
			getSearchResults={async (query) => {
				await new Promise((resolve) => {
					window.setTimeout(resolve, 400);
				});
				if (!query.toLowerCase().includes("dev")) {
					return [];
				}
				return [
					{
						value: "ws-1",
						label: "devbox",
						subtitle: "alice · docker",
						href: "/@alice/devbox",
					},
				];
			}}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "dev");
		await waitFor(() =>
			expect(body.getByRole("option", { name: /devbox/i })).toBeVisible(),
		);
		await expect(body.getByText("Workspaces")).toBeVisible();
		await expect(body.getByText("alice · docker")).toBeVisible();
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
		const body = bodyOf(canvasElement);
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
			expect(canvas.getByText("template:docker")).toBeVisible(),
		);

		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await waitFor(() => expect(body.getByText("Running")).toBeVisible());
		await userEvent.click(body.getByRole("option", { name: /Running/i }));
		await waitFor(() =>
			expect(canvas.getByText("status:running")).toBeVisible(),
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
		const body = bodyOf(canvasElement);
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
		await expect(canvas.getByText("owner:testuser01")).toBeVisible();
	},
};

// The Attributes category commits a distinct `key:true` chip per option, and
// several attribute chips can coexist because each owns its own key.
export const AttributesCommitBooleanChips: Story = {
	render: () => (
		<FilterComboboxHarness
			initialQuery=""
			categories={categoriesWithAttributes}
		/>
	),
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Attributes/i })).toBeVisible(),
		);
		await userEvent.click(body.getByRole("option", { name: /Attributes/i }));
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Outdated/i })).toBeVisible(),
		);
		await userEvent.click(body.getByRole("option", { name: /Outdated/i }));
		await waitFor(() =>
			expect(canvas.getByText("outdated:true")).toBeVisible(),
		);

		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await userEvent.click(body.getByRole("option", { name: /Attributes/i }));
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Shared/i })).toBeVisible(),
		);
		await userEvent.click(body.getByRole("option", { name: /Shared/i }));
		await waitFor(() => expect(canvas.getByText("shared:true")).toBeVisible());

		// Both boolean chips coexist because each attribute owns a distinct key.
		await expect(canvas.getByText("outdated:true")).toBeVisible();
		await expect(canvas.getByText("shared:true")).toBeVisible();
	},
};

// Escape closes the popup without clearing the committed chips.
export const DismissOnEscape: Story = {
	render: () => <FilterComboboxHarness initialQuery="owner:me" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
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
		await expect(canvas.getByText("owner:me")).toBeVisible();
	},
};

// Pressing outside the input dismisses the popup while keeping the chips.
export const DismissOnOutsideClick: Story = {
	render: () => <FilterComboboxHarness initialQuery="owner:me" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = bodyOf(canvasElement);
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
		await expect(canvas.getByText("owner:me")).toBeVisible();
	},
};

// A failed category lookup surfaces a Retry that refetches the options.
export const CategoryOptionsErrorRetry: Story = {
	render: () => {
		let attempts = 0;
		return (
			<FilterComboboxHarness
				initialQuery=""
				categories={[
					{
						key: "status",
						label: "Status",
						icon: <CircleDotIcon />,
						getOptions: async (query) => {
							attempts += 1;
							if (attempts === 1) {
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
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		const retry = await body.findByRole("button", { name: /retry/i });
		await userEvent.click(retry);
		await waitFor(() => expect(body.getByText("Running")).toBeVisible());
	},
};

// A failed suggestion lookup surfaces a Retry that refetches the typeahead.
export const TypeaheadErrorRetry: Story = {
	render: () => {
		let attempts = 0;
		return (
			<FilterComboboxHarness
				initialQuery=""
				categories={[
					{
						key: "owner",
						label: "Owner",
						icon: <UserIcon />,
						getOptions: async (query) => {
							attempts += 1;
							if (attempts === 1) {
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
		const body = bodyOf(canvasElement);
		const input = canvas.getByRole("combobox", {
			name: "Search and filter…",
		});
		await userEvent.click(input);
		await userEvent.type(input, "alice");
		const retry = await body.findByRole("button", { name: /retry/i });
		await userEvent.click(retry);
		await waitFor(() =>
			expect(body.getByRole("option", { name: /alice/i })).toBeVisible(),
		);
	},
};
