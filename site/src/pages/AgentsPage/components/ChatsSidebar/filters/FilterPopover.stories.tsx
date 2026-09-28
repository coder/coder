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
	decorators: [
		(Story) => (
			<div className="flex h-[540px] w-80 max-w-full justify-end">
				<Story />
			</div>
		),
	],
};

export default meta;
type Story = StoryObj<typeof FilterPopover>;

export const Closed: Story = {};

const openFilterMenu = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Filter agents" }),
	);
	return within(canvasElement.ownerDocument.body);
};

export const MenuOpen: Story = {
	play: async ({ canvasElement }) => {
		await openFilterMenu(canvasElement);
	},
};

const activeFilters: AgentSidebarFilters = {
	...DEFAULT_AGENT_SIDEBAR_FILTERS,
	prStatuses: ["draft", "open"],
	unread: true,
	sources: ["created_by_me"],
};

export const ActiveFilters: Story = {
	args: { filters: activeFilters },
	play: MenuOpen.play,
};

export const Archived: Story = {
	args: {
		filters: { ...DEFAULT_AGENT_SIDEBAR_FILTERS, archiveStatus: "archived" },
	},
	play: MenuOpen.play,
};

export const OwnerSubmenu: Story = {
	play: async ({ canvasElement }) => {
		const body = await openFilterMenu(canvasElement);
		await userEvent.click(body.getByRole("menuitem", { name: /^Owner/ }));
	},
};

export const FilterBySubmenu: Story = {
	args: { filters: activeFilters },
	play: async ({ canvasElement }) => {
		const body = await openFilterMenu(canvasElement);
		await userEvent.click(body.getByRole("menuitem", { name: /^Filter by/ }));
	},
};

export const ChatStatusFilters: Story = {
	args: { filters: { ...activeFilters, chatStatuses: ["error", "running"] } },
	play: MenuOpen.play,
};

export const GroupedBySubmenu: Story = {
	play: async ({ canvasElement }) => {
		const body = await openFilterMenu(canvasElement);
		await userEvent.click(body.getByRole("menuitem", { name: /^Grouped by/ }));
	},
};

export const KeyboardBadge: Story = {
	args: { filters: activeFilters },
	play: async ({ canvasElement }) => {
		const trigger = within(canvasElement).getByRole("button", {
			name: "Filter agents",
		});
		trigger.focus();
		await userEvent.keyboard(
			"{Enter}{ArrowDown}{ArrowDown}{ArrowDown}{ArrowDown}",
		);
	},
};

export const Mobile: Story = {
	globals: { viewport: { value: "iphone12", isRotated: false } },
	args: ActiveFilters.args,
	play: MenuOpen.play,
};

export const MobileFilterBy: Story = {
	globals: Mobile.globals,
	args: ActiveFilters.args,
	play: FilterBySubmenu.play,
};
