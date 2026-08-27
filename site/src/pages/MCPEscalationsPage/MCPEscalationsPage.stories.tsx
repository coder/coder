import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { API } from "#/api/api";
import type { MCPGatewayEscalation } from "#/api/typesGenerated";
import { mockApiError } from "#/testHelpers/entities";
import {
	MockMCPGatewayEscalation,
	MockNewerMCPGatewayEscalation,
} from "#/testHelpers/mcpGatewayEscalations";
import MCPEscalationsPage from "./MCPEscalationsPage";

const referenceDate = new Date("2026-08-25T12:00:00Z");

const meta = {
	title: "pages/MCPEscalationsPage/MCPEscalationsPage",
	component: MCPEscalationsPage,
	args: {
		referenceDate,
	},
	beforeEach: () => {
		spyOn(API, "getMCPGatewayEscalations").mockResolvedValue([
			MockMCPGatewayEscalation,
			MockNewerMCPGatewayEscalation,
		]);
		spyOn(API, "approveMCPGatewayEscalation").mockResolvedValue();
		spyOn(API, "denyMCPGatewayEscalation").mockResolvedValue();
	},
	parameters: {
		layout: "fullscreen",
	},
} satisfies Meta<typeof MCPEscalationsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPGatewayEscalations").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("status", {
				name: "Loading tool call approvals",
			}),
		).toBeVisible();
	},
};

export const Empty: Story = {
	beforeEach: () => {
		spyOn(API, "getMCPGatewayEscalations").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("No tool calls are waiting for approval"),
		).toBeVisible();
	},
};

export const PendingToolCalls: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const list = await canvas.findByRole("list", {
			name: "Pending tool call approvals",
		});

		// Scope to the pending list: the page header renders a heading too.
		const toolNames = within(list).getAllByRole("heading", { level: 2 });
		expect(toolNames).toHaveLength(2);
		expect(toolNames[0]).toHaveTextContent(MockNewerMCPGatewayEscalation.tool);
		expect(toolNames[1]).toHaveTextContent(MockMCPGatewayEscalation.tool);
		expect(canvas.getByText("linear")).toBeVisible();
		expect(canvas.getByText("agent-workspace")).toBeVisible();
		expect(
			canvas.getAllByText(/Expires in \d+ minutes\./).length,
		).toBeGreaterThan(0);

		const argumentsDisclosures = canvas.getAllByText("Arguments");
		const newerArguments = canvas.getByText(/"team": "ENG"/);
		expect(newerArguments).not.toBeVisible();
		await userEvent.click(argumentsDisclosures[0]);
		expect(newerArguments).toBeVisible();
	},
};

export const ApproveToolCall: Story = {
	beforeEach: () => {
		let escalations: MCPGatewayEscalation[] = [
			MockMCPGatewayEscalation,
			MockNewerMCPGatewayEscalation,
		];
		spyOn(API, "getMCPGatewayEscalations").mockImplementation(
			async () => escalations,
		);
		spyOn(API, "approveMCPGatewayEscalation").mockImplementation(async (id) => {
			escalations = escalations.filter((escalation) => escalation.id !== id);
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const approveButton = await canvas.findByRole("button", {
			name: `Approve ${MockNewerMCPGatewayEscalation.tool} tool call`,
		});

		await userEvent.click(approveButton);

		await waitFor(() =>
			expect(API.approveMCPGatewayEscalation).toHaveBeenCalledWith(
				MockNewerMCPGatewayEscalation.id,
			),
		);
		await waitFor(() =>
			expect(
				canvas.queryByRole("heading", {
					name: MockNewerMCPGatewayEscalation.tool,
				}),
			).not.toBeInTheDocument(),
		);
		expect(
			canvas.getByRole("heading", { name: MockMCPGatewayEscalation.tool }),
		).toBeVisible();
	},
};

export const ResolutionRace: Story = {
	beforeEach: () => {
		let isResolved = false;
		spyOn(API, "getMCPGatewayEscalations").mockImplementation(async () =>
			isResolved ? [] : [MockMCPGatewayEscalation],
		);
		spyOn(API, "approveMCPGatewayEscalation").mockImplementation(async () => {
			isResolved = true;
			throw {
				...mockApiError({
					message: "This tool call has already been resolved.",
				}),
				status: 409,
			};
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: `Approve ${MockMCPGatewayEscalation.tool} tool call`,
			}),
		);

		await waitFor(() =>
			expect(API.getMCPGatewayEscalations).toHaveBeenCalledTimes(2),
		);
		await expect(
			await canvas.findByText("This tool call has already been resolved."),
		).toBeVisible();
		await expect(
			canvas.findByText("No tool calls are waiting for approval"),
		).resolves.toBeVisible();
	},
};
