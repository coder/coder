import type { Meta, StoryObj } from "@storybook/react-vite";
import { CircleDotIcon, LayoutGridIcon, UserIcon } from "lucide-react";
import { useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { Avatar } from "#/components/Avatar/Avatar";
import type { UseFilterResult } from "#/components/Filter/Filter";
import { MockMenu } from "#/components/Filter/storyHelpers";
import { FilterCombobox } from "./FilterCombobox";
import type { FilterFacet } from "./types";

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
}: {
	initialQuery?: string;
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

export const TypeFacetPrefix: Story = {
	render: () => <FilterComboboxHarness initialQuery="" />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const input = canvas.getByPlaceholderText("Search and filter…");
		await userEvent.click(input);
		await userEvent.type(input, "status:");
		await expect(canvas.getByText("status:")).toBeVisible();
		await expect(canvas.getByText(/status:\s*Running/i)).toBeVisible();
	},
};
