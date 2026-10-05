import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { UserEngagementChart } from "./UserEngagementChart";

const meta: Meta<typeof UserEngagementChart> = {
	title: "pages/DeploymentSettingsPage/GeneralSettingsPage/UserEngagementChart",
	component: UserEngagementChart,
	args: {
		data: [
			{ date: "1/1/2024", users: 140 },
			{ date: "1/2/2024", users: 175 },
			{ date: "1/3/2024", users: 120 },
			{ date: "1/4/2024", users: 195 },
			{ date: "1/5/2024", users: 230 },
			{ date: "1/6/2024", users: 130 },
			{ date: "1/7/2024", users: 210 },
		],
	},
};

export default meta;
type Story = StoryObj<typeof UserEngagementChart>;

export const Loaded: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const chart = await canvas.findByRole("group", { name: "User engagement" });
		expect(chart).toHaveAttribute("aria-roledescription", "chart");
		// dom-accessibility-api doesn't compute descriptions from SVG <desc>
		// (browsers do, per SVG-AAM), so check the element itself.
		expect(chart.querySelector(":scope > desc")).toHaveTextContent(
			/^Daily engaged users from .+ to .+\. Use the left and right arrow keys/,
		);
		expect(canvas.queryByRole("application")).not.toBeInTheDocument();

		const liveRegion = canvas.getByRole("status");
		expect(liveRegion).toBeEmptyDOMElement();

		// Focusing the chart shows the first point; ArrowRight moves to the next.
		chart.focus();
		expect(chart).toHaveFocus();
		await waitFor(() => expect(liveRegion).toHaveTextContent(/^140 users /));
		await userEvent.keyboard("{ArrowRight}");
		await waitFor(() => expect(liveRegion).toHaveTextContent(/^175 users /));
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
