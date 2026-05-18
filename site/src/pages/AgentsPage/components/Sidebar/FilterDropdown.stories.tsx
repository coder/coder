import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FilterDropdown, SearchBar } from "./FilterDropdown";

const meta: Meta<typeof FilterDropdown> = {
	title: "pages/AgentsPage/FilterDropdown",
	component: FilterDropdown,
	args: {
		archivedFilter: "active",
		onArchivedFilterChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof FilterDropdown>;

export const OpensFilterMenu: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Filter agents" }),
		);

		await expect(
			await body.findByRole("menuitem", { name: /Active/i }),
		).toBeInTheDocument();
		await expect(
			await body.findByRole("menuitem", { name: /Archived/i }),
		).toBeInTheDocument();
	},
};

const searchMeta: Meta<typeof SearchBar> = {
	title: "pages/AgentsPage/SearchBar",
	component: SearchBar,
	args: {
		isOpen: false,
		onToggle: fn(),
		searchQuery: "",
		onSearchChange: fn(),
		resultCount: 0,
		totalCount: 12,
	},
};

type SearchStory = StoryObj<typeof SearchBar>;

export const SearchClosed: SearchStory = {
	...searchMeta,
	render: (args) => <SearchBar {...args} />,
};

export const SearchOpenEmpty: SearchStory = {
	...searchMeta,
	render: (args) => <SearchBar {...args} />,
	args: {
		...searchMeta.args,
		isOpen: true,
	},
};

export const SearchWithResults: SearchStory = {
	...searchMeta,
	render: (args) => <SearchBar {...args} />,
	args: {
		...searchMeta.args,
		isOpen: true,
		searchQuery: "Fix",
		resultCount: 4,
		totalCount: 12,
	},
};

export const OpensSearchInput: SearchStory = {
	...searchMeta,
	render: (args) => <SearchBar {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Search agents" }),
		).toBeInTheDocument();
	},
};
