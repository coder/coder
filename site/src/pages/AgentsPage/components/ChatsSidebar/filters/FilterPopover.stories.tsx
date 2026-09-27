import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { fn, userEvent, within } from "storybook/test";
import {
	type AgentSidebarFilters,
	DEFAULT_AGENT_SIDEBAR_FILTERS,
} from "../../../utils/agentSidebarFilters";
import { FilterPopover } from "./FilterPopover";

const meta: Meta<typeof FilterPopover> = {
	title: "pages/AgentsPage/FilterPopover",
	component: FilterPopover,
	args: {
		filters: DEFAULT_AGENT_SIDEBAR_FILTERS,
		onFiltersChange: fn(),
	},
	render: function FilterPopoverRender(args) {
		const [filters, setFilters] = useState(args.filters);
		return (
			<FilterPopover
				filters={filters}
				onFiltersChange={(nextFilters) => {
					setFilters(nextFilters);
					args.onFiltersChange(nextFilters);
				}}
			/>
		);
	},
};

export default meta;
type Story = StoryObj<typeof FilterPopover>;

export const Closed: Story = {};

const openFilterMenu = async (canvasElement: HTMLElement) => {
	const user = userEvent.setup();
	await user.click(
		within(canvasElement).getByRole("button", { name: "Filter agents" }),
	);
	return user;
};

export const MenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await openFilterMenu(canvasElement);
	},
};

const activeFilters = {
	archiveStatus: "archived",
	groupBy: "chat_status",
	prStatuses: ["draft", "open"],
	chatStatuses: ["unread"],
	sources: ["shared_with_me"],
} satisfies AgentSidebarFilters;

export const ActiveFilters: Story = {
	args: {
		filters: activeFilters,
	},
	play: async ({ canvasElement }) => {
		await openFilterMenu(canvasElement);
	},
};

export const PrStatusSubmenu: Story = {
	args: {
		filters: activeFilters,
	},
	play: async ({ canvasElement }) => {
		const user = await openFilterMenu(canvasElement);
		await user.click(
			await within(canvasElement.ownerDocument.body).findByRole("menuitem", {
				name: /PR status/,
			}),
		);
	},
};
