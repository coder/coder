import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentProps } from "react";
import { TemplateInsightsControls } from "./TemplateInsightsPage";

const meta: Meta<typeof TemplateInsightsControls> = {
	title: "pages/TemplatePage/TemplateInsightsControls",
	component: TemplateInsightsControls,
};

export default meta;
type Story = StoryObj<typeof TemplateInsightsControls>;

const defaultArgs: Partial<ComponentProps<typeof TemplateInsightsControls>> = {
	dateRange: { startDate: new Date(), endDate: new Date() },
	setDateRange: () => {},
	searchParams: new URLSearchParams(),
	setSearchParams: () => {},
};

export const Day: Story = {
	args: {
		...defaultArgs,
		interval: "day",
	},
};

export const Week: Story = {
	args: {
		...defaultArgs,
		interval: "week",
	},
};
