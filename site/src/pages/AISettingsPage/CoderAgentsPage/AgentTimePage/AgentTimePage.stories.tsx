import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useSearchParams } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import {
	MockAgentTimeMonthlyReport,
	MockAgentTimeNow,
	MockAgentTimeOrganizationOneId,
	MockAgentTimeOrganizationReport,
	MockAgentTimeReport,
	MockAgentTimeUserOneId,
} from "#/testHelpers/agentTime";
import { MockPermissions, MockUserOwner } from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import AgentTimePage from "./AgentTimePage";

const routePath = "/ai/settings/coder-agents/agent-time";

const AgentTimeSearchParamsProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return (
		<output aria-label="Agent time search params">
			{searchParams.toString() || "none"}
		</output>
	);
};

const withAgentTimeSearchParamsProbe: Decorator = (Story) => (
	<>
		<AgentTimeSearchParamsProbe />
		<Story />
	</>
);

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/AgentTimePage",
	component: AgentTimePage,
	decorators: [withAgentTimeSearchParamsProbe, withAuthProvider],
	args: {
		now: MockAgentTimeNow,
	},
	parameters: {
		user: MockUserOwner,
		permissions: {
			...MockPermissions,
			viewDeploymentConfig: true,
			editDeploymentConfig: false,
		},
		reactRouter: reactRouterParameters({
			location: { path: routePath },
			routing: { path: routePath },
		}),
	},
} satisfies Meta<typeof AgentTimePage>;

export default meta;
type Story = StoryObj<typeof AgentTimePage>;

export const AllHistoryDefaultsToMonthly: Story = {
	beforeEach: () => {
		spyOn(API, "getAgentTime").mockImplementation(async (request) => {
			if (request.interval !== "month") {
				throw new Error("Expected all history to use monthly buckets");
			}
			if (request.start_date !== undefined) {
				throw new Error("Expected all history to omit start_date");
			}
			if (request.end_date !== "2026-09-05") {
				throw new Error("Expected end_date to be the exclusive tomorrow date");
			}
			return MockAgentTimeMonthlyReport;
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", { name: "Agent time" }),
		).toBeVisible();
		await expect(canvas.getByText("All history to Sep 4, 2026")).toBeVisible();
	},
};

export const PresetUpdatesUrlState: Story = {
	beforeEach: () => {
		spyOn(API, "getAgentTime").mockResolvedValue(MockAgentTimeReport);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", { name: "Agent time" }),
		).toBeVisible();
		await userEvent.click(canvas.getByRole("button", { name: "Last 7 days" }));
		await expect(
			await canvas.findByText("Aug 29, 2026 to Sep 4, 2026"),
		).toBeVisible();
		await waitFor(() => {
			expect(
				canvas.getByLabelText("Agent time search params"),
			).toHaveTextContent("start_date=2026-08-29");
			expect(
				canvas.getByLabelText("Agent time search params"),
			).toHaveTextContent("end_date=2026-09-05");
		});
	},
};

export const OrganizationAndUserUrlState: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: {
				path: routePath,
				searchParams: {
					organization_id: MockAgentTimeOrganizationOneId,
					user_id: MockAgentTimeUserOneId,
					interval: "week",
				},
			},
			routing: { path: routePath },
		}),
	},
	beforeEach: () => {
		spyOn(API, "getAgentTime").mockResolvedValue(MockAgentTimeReport);
		spyOn(API, "getOrganizationAgentTime").mockImplementation(
			async (organization, request) => {
				if (organization !== MockAgentTimeOrganizationOneId) {
					throw new Error("Expected organization route parameter");
				}
				if (request.organization_id !== undefined) {
					throw new Error(
						"Expected organization_id to stay out of org request",
					);
				}
				if (request.user_id !== MockAgentTimeUserOneId) {
					throw new Error("Expected user_id from the URL state");
				}
				return MockAgentTimeOrganizationReport;
			},
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("heading", { name: "Users" }),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: "All organizations" }),
		);
		await waitFor(() => {
			const output = canvas.getByLabelText("Agent time search params");
			expect(output).not.toHaveTextContent("organization_id=");
			expect(output).not.toHaveTextContent("user_id=");
		});
	},
};

export const DeploymentUsers: Story = {
	beforeEach: () => {
		spyOn(API, "getAgentTime").mockImplementation(async (request) =>
			request.group_by === "user"
				? MockAgentTimeOrganizationReport
				: MockAgentTimeReport,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(await canvas.findByRole("button", { name: "Users" }));
		await expect(
			await canvas.findByRole("heading", { name: "Users" }),
		).toBeVisible();
		await userEvent.click(
			within(canvas.getByRole("row", { name: /Alice Ng/ })).getByRole(
				"button",
				{ name: "View user" },
			),
		);
		await waitFor(() => {
			expect(
				canvas.getByLabelText("Agent time search params"),
			).toHaveTextContent("group_by=user");
			expect(
				canvas.getByLabelText("Agent time search params"),
			).toHaveTextContent(`user_id=${MockAgentTimeUserOneId}`);
			expect(
				canvas.getByLabelText("Agent time search params"),
			).not.toHaveTextContent("organization_id=");
		});
		await userEvent.click(canvas.getByRole("button", { name: /Clear user/ }));
		await userEvent.click(
			canvas.getByRole("button", { name: "Organizations" }),
		);
		await expect(
			await canvas.findByRole("heading", {
				name: "Organizations",
			}),
		).toBeVisible();
		await expect(
			canvas.getByLabelText("Agent time search params"),
		).not.toHaveTextContent("user_id=");
	},
};
