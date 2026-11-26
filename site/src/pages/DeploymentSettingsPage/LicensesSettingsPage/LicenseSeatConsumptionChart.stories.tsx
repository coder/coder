import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LicenseSeatConsumptionChart } from "./LicenseSeatConsumptionChart";

const meta: Meta<typeof LicenseSeatConsumptionChart> = {
	title:
		"pages/DeploymentSettingsPage/LicensesSettingsPage/LicenseSeatConsumptionChart",
	component: LicenseSeatConsumptionChart,
	args: {
		limit: 220,
		data: [
			{ date: "1/1/2024", users: 150 },
			{ date: "1/2/2024", users: 165 },
			{ date: "1/3/2024", users: 180 },
			{ date: "1/4/2024", users: 155 },
			{ date: "1/5/2024", users: 190 },
			{ date: "1/6/2024", users: 200 },
			{ date: "1/7/2024", users: 210 },
		],
	},
};

export default meta;
type Story = StoryObj<typeof LicenseSeatConsumptionChart>;

export const Loaded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chart = await canvas.findByRole("group", {
			name: "License seat consumption",
		});
		expect(chart).toHaveAttribute("aria-roledescription", "chart");
		expect(canvas.queryByRole("application")).not.toBeInTheDocument();
	},
};

// Highlights the second point from the keyboard, so its named active dot and
// tooltip are captured.
export const KeyboardHighlight: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chart = await canvas.findByRole("group", {
			name: "License seat consumption",
		});
		chart.focus();
		await userEvent.keyboard("{ArrowRight}");
		await canvas.findByRole("img", { name: /^165 seats, / });
	},
};

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
