import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import {
	MockAgentTimeOrganizationOneId,
	MockAgentTimeOrganizationReport,
	MockAgentTimeReport,
	MockAgentTimeUserOneId,
} from "#/testHelpers/agentTime";
import { BreakdownTable } from "./AgentTimeBreakdownTable";
import { AgentTimeChart } from "./AgentTimeChart";

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/AgentTimeComponents",
	component: BreakdownTable,
	args: {
		query: {
			data: MockAgentTimeReport,
			isPlaceholderData: false,
			currentPage: 1,
			limit: 25,
			onPageChange: fn(),
			goToPreviousPage: fn(),
			goToNextPage: fn(),
			goToFirstPage: fn(),
			isSuccess: true,
			hasNextPage: false,
			hasPreviousPage: false,
			totalRecords: 3,
			totalPages: 1,
			currentOffsetStart: 1,
			countIsCapped: false,
		},
		tableGroup: "organization",
		totalAgentTimeMs: MockAgentTimeReport.total_agent_time_ms,
		sortBy: "agent_time",
		sortOrder: "desc",
		onSortChange: fn(),
		onSelectOrganization: fn(),
		onSelectUser: fn(),
	},
} satisfies Meta<typeof BreakdownTable>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Organizations: Story = {
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		const row = canvas.getByRole("row", { name: /Acme Engineering/ });
		await userEvent.click(
			within(row).getByRole("button", { name: "View users" }),
		);
		await expect(args.onSelectOrganization).toHaveBeenCalledWith(
			MockAgentTimeOrganizationOneId,
		);
		await userEvent.click(canvas.getByRole("button", { name: "Agent time" }));
		await expect(args.onSortChange).toHaveBeenCalledWith("agent_time");
	},
};
export const Users: Story = {
	args: {
		query: { ...meta.args.query, data: MockAgentTimeOrganizationReport },
		tableGroup: "user",
		selectedUserId: MockAgentTimeUserOneId,
	},
	play: async ({ args, canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			within(canvas.getByRole("row", { name: /Alice Ng/ })).getByRole(
				"button",
				{ name: "Selected" },
			),
		).toBeDisabled();
		await userEvent.click(
			within(canvas.getByRole("row", { name: /Bob Stone/ })).getByRole(
				"button",
				{ name: "View user" },
			),
		);
		await expect(args.onSelectUser).toHaveBeenCalled();
	},
};
export const Chart: Story = {
	render: () => <AgentTimeChart report={MockAgentTimeReport} />,
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByRole("application")).toBeVisible();
		await expect(await canvas.findByText("12h")).toBeVisible();
	},
};
