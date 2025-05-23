import type { Meta, StoryObj } from "@storybook/react";
import { sub } from "date-fns";
import { LastSeen } from "./LastSeen";

const meta: Meta<typeof LastSeen> = {
	title: "components/LastSeen",
	component: LastSeen,
	args: {},
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
