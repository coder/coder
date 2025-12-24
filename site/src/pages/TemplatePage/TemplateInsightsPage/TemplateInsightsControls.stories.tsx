import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "@testing-library/react";
import type { ComponentProps } from "react";
import { userEvent } from "storybook/test";
import { TemplateInsightsControls } from "./TemplateInsightsPage";

const meta: Meta<typeof TemplateInsightsControls> = {
	title: "pages/TemplatePage/TemplateInsightsControls",
	component: TemplateInsightsControls,
};

export default meta;
type Story = StoryObj<typeof TemplateInsightsControls>;

const defaultArgs: Partial<ComponentProps<typeof TemplateInsightsControls>> = {
	dateRange: {
		startDate: new Date("2025-08-05"),
		endDate: new Date("2025-08-07"),
	},
	setDateRange: () => {},
	searchParams: new URLSearchParams(),
	setSearchParams: () => {},
};

export const Day: Story = {
	args: {
		...defaultArgs,
		interval: "day",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const datePicker = canvas.getAllByRole("button")[1]!;
		await userEvent.click(datePicker);
	},
};

export const Week: Story = {
	args: {
		...defaultArgs,
		interval: "week",
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const dropdown = canvas.getAllByRole("button")[1]!;
		await userEvent.click(dropdown);
	},
};
