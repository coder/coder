import { MockInterception } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { RequestLogsPageView } from "./RequestLogsPageView";

const meta: Meta<typeof RequestLogsPageView> = {
	title: "pages/AIGovernancePage/RequestLogsPageView",
	component: RequestLogsPageView,
	args: {},
};

export default meta;
type Story = StoryObj<typeof RequestLogsPageView>;

export const NotEnabled: Story = {
	args: {
		isRequestLogsVisible: false,
	},
};

export const WithLogs: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions: [MockInterception],
	},
};

export const EmptyLogs: Story = {
	args: {
		isRequestLogsVisible: true,
		interceptions: [],
	},
};
