import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	type AgentSidebarFilters,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { FilterMenu } from "./FilterMenu";

const meta: Meta<typeof FilterMenu> = {
	title: "pages/AgentsPage/FilterMenu",
	component: FilterMenu,
	args: {
		filters: DEFAULT_AGENT_SIDEBAR_FILTERS,
		onFiltersChange: fn(),
	},
	render: function FilterMenuRender(args) {
		const [filters, setFilters] = useState(args.filters);
		return (
			<FilterMenu
				filters={filters}
				onFiltersChange={(nextFilters) => {
					setFilters(nextFilters);
					args.onFiltersChange(nextFilters);
				}}
			/>
		);
	},
	decorators: [
		(Story) => (
			<div className="flex h-[480px] w-[360px] justify-end">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof FilterMenu>;

const openMenu = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Filter agents" }),
	);
	return within(
		await within(document.body).findByRole("menu", { name: "Filter agents" }),
	);
};

const activeFilters: AgentSidebarFilters = {
	...DEFAULT_AGENT_SIDEBAR_FILTERS,
	sources: ["created_by_me"],
	timeRange: "30d",
	prStatuses: ["draft", "open"],
	chatStatuses: ["unread"],
};

export const Defaults: Story = {
	play: async ({ canvasElement }) => {
		await openMenu(canvasElement);
	},
};

export const WithActiveFilters: Story = {
	args: {
		filters: activeFilters,
	},
	play: async ({ canvasElement }) => {
		await openMenu(canvasElement);
	},
};

export const GroupedBySubmenu: Story = {
	play: async ({ canvasElement }) => {
		const menu = await openMenu(canvasElement);
		await userEvent.click(menu.getByRole("menuitem", { name: /Grouped by/ }));
	},
};

export const TimeRangeSubmenu: Story = {
	args: {
		filters: activeFilters,
	},
	play: async ({ canvasElement }) => {
		const menu = await openMenu(canvasElement);
		await userEvent.click(menu.getByRole("menuitem", { name: /Time range/ }));
	},
};

export const FilterBySubmenu: Story = {
	args: {
		filters: activeFilters,
	},
	play: async ({ canvasElement }) => {
		const menu = await openMenu(canvasElement);
		await userEvent.click(menu.getByRole("menuitem", { name: /Filter by/ }));
	},
};
