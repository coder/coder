import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { DEFAULT_FILTER_STATE, FilterDropdown } from "./FilterDropdown";

const meta: Meta<typeof FilterDropdown> = {
	title: "pages/AgentsPage/FilterDropdown",
	component: FilterDropdown,
	args: {
		archivedFilter: "active",
		onArchivedFilterChange: fn(),
		filterState: DEFAULT_FILTER_STATE,
		onFilterStateChange: fn(),
		filteredCount: 1,
		totalRootCount: 1,
		prStatusCounts: new Map([["open", 1]]),
		chatStatusCounts: new Map([["idle", 1]]),
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

		await expect(await body.findByText("Show")).toBeInTheDocument();
		await expect(await body.findByText("Active agents")).toBeInTheDocument();
		await expect(await body.findByText("Archived agents")).toBeInTheDocument();
		await expect(await body.findByText("Group")).toBeInTheDocument();
		await expect(await body.findByText("PR status")).toBeInTheDocument();
	},
};
