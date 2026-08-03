import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleDotIcon, LayoutGridIcon, UserIcon } from "lucide-react";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { MockMenu } from "#/components/Filter/storyHelpers";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterFacet, FilterSearchResult } from "./types";

const meta: Meta<typeof FilterCombobox> = {
	title: "components/Filter/FilterCombobox",
	component: FilterCombobox,
};

export default meta;
type Story = StoryObj<typeof FilterCombobox>;

const statusMenu = {
	...MockMenu,
	searchOptions: [
		{ label: "Running", value: "running" },
		{ label: "Stopped", value: "stopped" },
	],
};

const ownerMenu = {
	...MockMenu,
	searchOptions: [
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
	],
};

const facets: FilterFacet[] = [
	{ id: "status", label: "Status", icon: CircleDotIcon, menu: statusMenu },
	{ id: "template", label: "Template", icon: LayoutGridIcon, menu: MockMenu },
	{
		id: "owner",
		label: "Owner",
		aliases: ["user"],
		icon: UserIcon,
		menu: ownerMenu,
	},
];

const FilterComboboxHarness = ({
	initialQuery = "owner:me",
	getSearchResults,
	onSearchResultSelect,
	searchResultsLabel,
}: {
	initialQuery?: string;
	getSearchResults?: (query: string) => Promise<FilterSearchResult[]>;
	onSearchResultSelect?: (result: FilterSearchResult) => void;
	searchResultsLabel?: string;
}) => {
	const [query, setQuery] = useState(initialQuery);
	const values = Object.fromEntries(
		[...query.matchAll(/(\w+):"([^"]+)"|(\w+):(\S+)/g)].map((match) => [
			match[1] ?? match[3],
			match[2] ?? match[4],
		]),
	);

	const filter: UseFilterResult = {
		query,
		values,
		used: query !== "" && query !== "owner:me",
		update: (next) => {
			setQuery(typeof next === "string" ? next : query);
		},
		debounceUpdate: (next) => {
			setQuery(typeof next === "string" ? next : query);
		},
		cancelDebounce: () => {},
	};

	return (
		<FilterCombobox
			filter={filter}
			facets={facets}
			chipKeys={["owner", "status", "template"]}
			placeholder="Search and filter…"
			className="max-w-lg"
			getSearchResults={getSearchResults}
			onSearchResultSelect={onSearchResultSelect}
			searchResultsLabel={searchResultsLabel}
		/>
	);
};

export const Default: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByPlaceholderText("Search and filter…"),
		).toBeVisible();
		await expect(canvas.getByText("owner:me")).toBeVisible();
	},
};

export const OpenFilterMenu: Story = {
	render: () => <FilterComboboxHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", { name: "Toggle filters" }),
		);
		await expect(canvas.getByText("Filter by")).toBeVisible();
		await expect(canvas.getByRole("button", { name: /Status/i })).toBeVisible();
	},
};

export const FocusShowsCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await expect(canvas.getByRole("option", { name: /Owner/i })).toBeVisible();
		await expect(canvas.getByRole("option", { name: /Status/i })).toBeVisible();
		await expect(
			canvas.getByRole("option", { name: /Template/i }),
		).toBeVisible();
	},
};

export const TypeFacetPrefix: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await expect(canvas.getByText("status:")).toBeVisible();
		await expect(canvas.getByText("Running")).toBeVisible();
	},
};

export const TypeaheadMatchingCategories: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await userEvent.type(input, "ow");
		await expect(canvas.getByRole("option", { name: /Owner/i })).toBeVisible();
		await expect(
			canvas.queryByRole("option", { name: /Status/i }),
		).not.toBeInTheDocument();
		await userEvent.keyboard("{Enter}");
		await expect(canvas.getByText("owner:")).toBeVisible();
		await expect(canvas.getByText("alice")).toBeVisible();
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
						id: "ws-1",
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
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await userEvent.type(input, "dev");
		await expect(canvas.getByText("Workspaces")).toBeVisible();
		await waitFor(() =>
			expect(canvas.getByRole("option", { name: /devbox/i })).toBeVisible(),
		);
		await expect(canvas.getByText("alice · docker")).toBeVisible();
	},
};

const CrossCategoryValueSuggestionsHarness = () => {
	const [ownerQuery, setOwnerQuery] = useState("");
	const ownerOptions = [
		{ label: "testuser01", value: "testuser01" },
		{ label: "alice", value: "alice" },
	].filter(
		(option) =>
			option.label.toLowerCase().includes(ownerQuery.toLowerCase()) ||
			option.value.toLowerCase().includes(ownerQuery.toLowerCase()),
	);

	const crossFacets: FilterFacet[] = [
		{ id: "status", label: "Status", icon: CircleDotIcon, menu: statusMenu },
		{
			id: "owner",
			label: "Owner",
			aliases: ["user"],
			icon: UserIcon,
			menu: {
				...MockMenu,
				setQuery: setOwnerQuery,
				searchOptions: ownerOptions,
			},
		},
	];

	const [query, setQuery] = useState("");
	const values = Object.fromEntries(
		[...query.matchAll(/(\w+):"([^"]+)"|(\w+):(\S+)/g)].map((match) => [
			match[1] ?? match[3],
			match[2] ?? match[4],
		]),
	);

	const filter: UseFilterResult = {
		query,
		values,
		used: query !== "",
		update: (next) => {
			setQuery(typeof next === "string" ? next : query);
		},
		debounceUpdate: (next) => {
			setQuery(typeof next === "string" ? next : query);
		},
		cancelDebounce: () => {},
	};

	return (
		<FilterCombobox
			filter={filter}
			facets={crossFacets}
			chipKeys={["owner", "status"]}
			placeholder="Search and filter…"
			className="max-w-lg"
		/>
	);
};

export const CrossCategoryValueSuggestions: Story = {
	render: () => <CrossCategoryValueSuggestionsHarness />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await userEvent.type(input, "test");
		await expect(canvas.getByText("Owner")).toBeVisible();
		await expect(
			canvas.getByRole("option", { name: /testuser01/i }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("option", { name: /^Owner$/i }),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("option", { name: /testuser01/i }));
		await expect(canvas.getByText("owner:testuser01")).toBeVisible();
	},
};
