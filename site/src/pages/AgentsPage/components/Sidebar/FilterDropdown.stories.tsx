import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import { FilterDropdown } from "./FilterDropdown";

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

		await expect(body.getByRole("menuitem", { name: /Active/i })).toBeVisible();
		await expect(
			body.getByRole("menuitem", { name: /Archived/i }),
		).toBeVisible();
	},
};
