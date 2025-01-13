import type { Meta, StoryObj } from "@storybook/react";
import { UsersCountChart } from "./UsersCountChart";

const meta: Meta<typeof UsersCountChart> = {
	title: "pages/DeploymentSettingsPage/GeneralSettingsPage/UsersCountChart",
	component: UsersCountChart,
	args: {
		active: [
			{ date: "1/1/2024", amount: 150 },
			{ date: "1/2/2024", amount: 165 },
			{ date: "1/3/2024", amount: 180 },
			{ date: "1/4/2024", amount: 155 },
			{ date: "1/5/2024", amount: 190 },
			{ date: "1/6/2024", amount: 200 },
			{ date: "1/7/2024", amount: 210 },
		],
		dormant: [
			{ date: "1/1/2024", amount: 80 },
			{ date: "1/2/2024", amount: 82 },
			{ date: "1/3/2024", amount: 85 },
			{ date: "1/4/2024", amount: 88 },
			{ date: "1/5/2024", amount: 90 },
			{ date: "1/6/2024", amount: 92 },
			{ date: "1/7/2024", amount: 95 },
		],
		suspended: [
			{ date: "1/1/2024", amount: 20 },
			{ date: "1/2/2024", amount: 22 },
			{ date: "1/3/2024", amount: 25 },
			{ date: "1/4/2024", amount: 23 },
			{ date: "1/5/2024", amount: 28 },
			{ date: "1/6/2024", amount: 30 },
			{ date: "1/7/2024", amount: 32 },
		],
	},
};

export default meta;
type Story = StoryObj<typeof UsersCountChart>;

export const Default: Story = {};
