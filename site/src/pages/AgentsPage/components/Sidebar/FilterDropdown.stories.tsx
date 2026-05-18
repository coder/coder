import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DEFAULT_FILTER_STATE, FilterDropdown } from "./FilterDropdown";

const meta: Meta<typeof FilterDropdown> = {
	title: "pages/AgentsPage/FilterDropdown",
	component: FilterDropdown,
	args: {
		filterState: DEFAULT_FILTER_STATE,
		onFilterChange: fn(),
	},
};

export default meta;
type Story = StoryObj<typeof FilterDropdown>;

export const Default: Story = {};

export const WithActiveFilters: Story = {
	args: {
		filterState: {
			groupBy: "chat_status",
			prStatus: new Set(["open", "draft"]),
			unread: true,
		},
	},
};

export const OpensFilterPanel: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(document.body);

		await userEvent.click(
			canvas.getByRole("button", { name: "Filter agents" }),
		);

		await expect(await body.findByText("Group")).toBeInTheDocument();
		await expect(await body.findByText("Filter by")).toBeInTheDocument();
		await expect(await body.findByText("PR status")).toBeInTheDocument();
		await expect(await body.findByText("Chat status")).toBeInTheDocument();
		await expect(
			await body.findByRole("button", { name: /Apply/i }),
		).toBeInTheDocument();
		await expect(await body.findByText("Clear all")).toBeInTheDocument();
	},
};
