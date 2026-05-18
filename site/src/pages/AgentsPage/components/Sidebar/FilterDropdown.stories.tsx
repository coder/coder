import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FilterDropdown, SearchButton } from "./FilterDropdown";

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

const searchButtonMeta: Meta<typeof SearchButton> = {
	title: "pages/AgentsPage/SearchButton",
	component: SearchButton,
	args: {
		onClick: fn(),
	},
};

type SearchButtonStory = StoryObj<typeof SearchButton>;

export const SearchButtonDefault: SearchButtonStory = {
	...searchButtonMeta,
	render: (args) => <SearchButton {...args} />,
};

export const ClicksSearchButton: SearchButtonStory = {
	...searchButtonMeta,
	render: (args) => <SearchButton {...args} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", { name: "Search agents" }),
		).toBeInTheDocument();
	},
};
