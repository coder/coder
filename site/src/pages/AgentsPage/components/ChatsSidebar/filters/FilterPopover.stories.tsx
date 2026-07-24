import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, fn, userEvent, within } from "storybook/test";
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
	render: (args) => {
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

const openFilterDialog = async (canvasElement: HTMLElement) => {
	await userEvent.click(
		within(canvasElement).getByRole("button", { name: "Filter agents" }),
	);
	return within(
		await within(document.body).findByRole("dialog", {
			name: "Filter agents",
		}),
	);
};

export const AppliesStagedFilters: Story = {
	args: {
		onFiltersChange: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const dialog = await openFilterDialog(canvasElement);

		await userEvent.click(dialog.getByRole("radio", { name: "Chat status" }));
		await userEvent.click(dialog.getByRole("checkbox", { name: "Draft" }));
		await userEvent.click(dialog.getByRole("checkbox", { name: "Read" }));

		expect(args.onFiltersChange).not.toHaveBeenCalled();

		await userEvent.click(dialog.getByRole("button", { name: "Apply" }));

		await expect(args.onFiltersChange).toHaveBeenCalledWith({
			archiveStatus: "active",
			groupBy: "chat_status",
			prStatuses: ["draft"],
			chatStatuses: ["unread"],
			sources: ["created_by_me"],
		});
	},
};

export const KeepsOneChatStatusSelected: Story = {
	args: {
		filters: {
			archiveStatus: "active",
			groupBy: "date",
			prStatuses: [],
			chatStatuses: ["unread"],
			sources: ["created_by_me"],
		} satisfies AgentSidebarFilters,
		onFiltersChange: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const dialog = await openFilterDialog(canvasElement);

		await userEvent.click(dialog.getByRole("checkbox", { name: "Unread" }));

		expect(dialog.getByRole("checkbox", { name: "Unread" })).toBeChecked();
		expect(dialog.getByRole("checkbox", { name: "Read" })).not.toBeChecked();

		await userEvent.click(dialog.getByRole("button", { name: "Apply" }));

		await expect(args.onFiltersChange).toHaveBeenCalledWith({
			archiveStatus: "active",
			groupBy: "date",
			prStatuses: [],
			chatStatuses: ["unread"],
			sources: ["created_by_me"],
		});
	},
};

export const KeepsOneSourceSelected: Story = {
	args: {
		filters: {
			archiveStatus: "active",
			groupBy: "date",
			prStatuses: [],
			chatStatuses: ["unread", "read"],
			sources: ["shared_with_me"],
		} satisfies AgentSidebarFilters,
		onFiltersChange: fn(),
	},
	play: async ({ args, canvasElement }) => {
		const dialog = await openFilterDialog(canvasElement);

		await userEvent.click(
			dialog.getByRole("checkbox", { name: "Shared with me" }),
		);

		expect(
			dialog.getByRole("checkbox", { name: "Created by me" }),
		).not.toBeChecked();
		expect(
			dialog.getByRole("checkbox", { name: "Shared with me" }),
		).toBeChecked();

		await userEvent.click(dialog.getByRole("button", { name: "Apply" }));

		await expect(args.onFiltersChange).toHaveBeenCalledWith({
			archiveStatus: "active",
			groupBy: "date",
			prStatuses: [],
			chatStatuses: ["unread", "read"],
			sources: ["shared_with_me"],
		});
	},
};
