import type { Meta, StoryObj } from "@storybook/react-vite";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";

const meta: Meta<typeof LicenseSeatConsumptionChart> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/LicenseSeatConsumptionChart",
	component: LicenseSeatConsumptionChart,
	args: {
		data: [
			{ date: "1/1/2024", users: 10, limit: undefined },
			{ date: "1/2/2024", users: 15, limit: undefined },
			{ date: "1/3/2024", users: 20, limit: undefined },
			{ date: "1/4/2024", users: 30, limit: 50 },
			{ date: "1/5/2024", users: 35, limit: 50 },
			{ date: "1/6/2024", users: 40, limit: 50 },
			{ date: "1/7/2024", users: 45, limit: 50 },
			{ date: "1/8/2024", users: 55, limit: 100 },
			{ date: "1/9/2024", users: 60, limit: 100 },
			{ date: "1/10/2024", users: 65, limit: 100 },
			{ date: "1/11/2024", users: 68, limit: 75 },
			{ date: "1/12/2024", users: 72, limit: 75 },
		],
	},
};

export default meta;
type Story = StoryObj<typeof LicenseSeatConsumptionChart>;

export const Loaded: Story = {};

export const Empty: Story = {
	args: {
		data: [],
	},
};

export const Loading: Story = {
	args: {
		data: undefined,
	},
};
