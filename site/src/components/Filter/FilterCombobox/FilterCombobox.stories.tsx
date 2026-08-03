import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleDotIcon, LayoutGridIcon, UserIcon } from "lucide-react";
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

/** Combobox popup is portaled to document.body, outside the Storybook canvas. */
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
// Categories render as status, template, owner; arrowing to the second row
// and pressing Enter must open Template, not Status.
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
		await userEvent.keyboard("{ArrowDown}");
		await waitFor(() =>
			expect(body.getByRole("option", { name: /Template/i })).toHaveAttribute(
				"data-highlighted",
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
