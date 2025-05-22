import type { Meta, StoryObj } from "@storybook/react";
import { sub } from "date-fns";
import { LastSeen } from "./LastSeen";

const meta: Meta<typeof LastSeen> = {
	title: "components/LastSeen",
	component: LastSeen,
	args: {
		// We typically want this component to be excluded from Chromatic's snapshots,
		// because it creates a lot of noise when a static dates roles over from eg.
		// "2 months ago" to "3 months ago", but these stories use relative dates,
		// and test specific cases that we want to be validated.
		"data-chromatic": "",
	},
};

export default meta;
type Story = StoryObj<typeof LastSeen>;

export const Now: Story = {
	args: {
		at: new Date(),
	},
};

export const OneDayAgo: Story = {
	args: {
		at: sub(new Date(), { days: 1 }),
	},
};

export const OneWeekAgo: Story = {
	args: {
		at: sub(new Date(), { weeks: 1 }),
	},
};

export const OneMonthAgo: Story = {
	args: {
		at: sub(new Date(), { months: 1 }),
	},
};

export const OneYearAgo: Story = {
	args: {
		at: sub(new Date(), { years: 1 }),
	},
};

export const Never: Story = {
	args: {
		at: sub(new Date(), { years: 101 }),
	},
};
