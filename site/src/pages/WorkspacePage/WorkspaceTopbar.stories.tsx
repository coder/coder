import type { Meta, StoryObj } from "@storybook/react";
import { expect, screen, userEvent, waitFor, within } from "@storybook/test";
import { getWorkspaceQuotaQueryKey } from "api/queries/workspaceQuota";
import type { Workspace, WorkspaceQuota } from "api/typesGenerated";
import { addHours, addMinutes } from "date-fns";
import {
	MockOrganization,
	MockTemplate,
	MockTemplateVersion,
	MockUser,
	MockWorkspace,
} from "testHelpers/entities";
import { withAuthProvider, withDashboardProvider } from "testHelpers/storybook";
import { WorkspaceTopbar } from "./WorkspaceTopbar";

// We want a workspace without a deadline to not pollute the screenshot. Also
// want to make sure that the workspace is synced to our other mock values
const baseWorkspace: Workspace = {
	...MockWorkspace,
	organization_name: MockOrganization.name,
	organization_id: MockOrganization.id,
	latest_build: {
		...MockWorkspace.latest_build,
		deadline: undefined,
	},
};

const meta: Meta<typeof WorkspaceTopbar> = {
	title: "pages/WorkspacePage/WorkspaceTopbar",
	component: WorkspaceTopbar,
	decorators: [withAuthProvider, withDashboardProvider],
	args: {
		workspace: baseWorkspace,
		template: MockTemplate,
		latestVersion: MockTemplateVersion,
		canUpdateWorkspace: true,
	},
	parameters: {
		layout: "fullscreen",
		features: ["advanced_template_scheduling"],
		chromatic: {
			diffThreshold: 0.6,
		},
		user: MockUser,
	},
};

export default meta;
type Story = StoryObj<typeof WorkspaceTopbar>;

export const Example: Story = {};

export const Outdated: Story = {
	args: {
		workspace: {
			...baseWorkspace,
			outdated: true,
		},
	},
};

export const ReadyWithDeadline: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 8).toISOString();
				},
			},
		},
	},
};

export const Connected: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			get last_used_at() {
				return new Date().toISOString();
			},
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 8).toISOString();
				},
			},
		},
	},
	play: async ({ canvasElement, step }) => {
		const screen = within(canvasElement);
		const autostopText = "Stop in 8 hours";

		await step("show controls", async () => {
			await userEvent.click(screen.getByTestId("schedule-icon-button"));
			await waitFor(() =>
				expect(screen.getByText(autostopText)).toBeInTheDocument(),
			);
		});

		await step("hide controls", async () => {
			await userEvent.click(screen.getByTestId("schedule-icon-button"));
			await waitFor(() =>
				expect(screen.queryByText(autostopText)).not.toBeInTheDocument(),
			);
		});
	},
};
export const ConnectedWithMaxDeadline: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			get last_used_at() {
				return new Date().toISOString();
			},
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 1).toISOString();
				},
				get max_deadline() {
					return addHours(new Date(), 8).toISOString();
				},
			},
		},
	},
	play: async ({ canvasElement, step }) => {
		const screen = within(canvasElement);
		const autostopText = "Stop in an hour";

		await step("show controls", async () => {
			await userEvent.click(screen.getByTestId("schedule-icon-button"));
			await waitFor(() =>
				expect(screen.getByText(autostopText)).toBeInTheDocument(),
			);
		});

		await step("hide controls", async () => {
			await userEvent.click(screen.getByTestId("schedule-icon-button"));
			await waitFor(() =>
				expect(screen.queryByText(autostopText)).not.toBeInTheDocument(),
			);
		});
	},
};
export const ConnectedWithMaxDeadlineSoon: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			get last_used_at() {
				return new Date().toISOString();
			},
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 1).toISOString();
				},
				get max_deadline() {
					return addHours(new Date(), 1).toISOString();
				},
			},
		},
	},
};

export const Dormant: Story = {
	args: {
		workspace: {
			...baseWorkspace,
			deleting_at: new Date("2024/01/01").toISOString(),
			latest_build: {
				...baseWorkspace.latest_build,
				status: "failed",
			},
		},
	},
};

export const WithExceededDeadline: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				deadline: MockWorkspace.latest_build.deadline,
			},
		},
	},
};

export const WithApproachingDeadline: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addMinutes(new Date(), 30).toISOString();
				},
			},
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("activate hover trigger", async () => {
			await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					/this workspace has enabled autostop/,
				),
			);
		});
	},
};

export const WithFarAwayDeadline: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 8).toISOString();
				},
			},
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("activate hover trigger", async () => {
			await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					/this workspace has enabled autostop/,
				),
			);
		});
	},
};

export const WithFarAwayDeadlineRequiredByTemplate: Story = {
	args: {
		workspace: {
			...MockWorkspace,
			latest_build: {
				...MockWorkspace.latest_build,
				get deadline() {
					return addHours(new Date(), 8).toISOString();
				},
			},
		},
		template: {
			...MockTemplate,
			allow_user_autostop: false,
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("activate hover trigger", async () => {
			await userEvent.hover(canvas.getByTestId("schedule-controls-autostop"));
			await waitFor(() =>
				expect(screen.getByRole("tooltip")).toHaveTextContent(
					/template has an autostop requirement/,
				),
			);
		});
	},
};

export const WithQuotaNoOrgs: Story = {
	parameters: {
		showOrganizations: false,
		queries: [
			{
				key: getWorkspaceQuotaQueryKey(
					MockOrganization.name,
					MockUser.username,
				),
				data: {
					credits_consumed: 2,
					budget: 40,
				} satisfies WorkspaceQuota,
			},
		],
	},
};

export const WithQuotaWithOrgs: Story = {
	parameters: {
		showOrganizations: true,
		queries: [
			{
				key: getWorkspaceQuotaQueryKey(
					MockOrganization.name,
					MockUser.username,
				),
				data: {
					credits_consumed: 2,
					budget: 40,
				} satisfies WorkspaceQuota,
			},
		],
	},
};

export const TemplateInfoPopover: Story = {
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("activate hover trigger", async () => {
			await userEvent.hover(canvas.getByText(baseWorkspace.name));
			await waitFor(() =>
				expect(
					canvas.getByRole("presentation", { hidden: true }),
				).toHaveTextContent(MockTemplate.display_name),
			);
		});
	},
	parameters: {
		chromatic: {
			diffThreshold: 0.6,
		},
	},
};

export const TemplateInfoPopoverWithoutDisplayName: Story = {
	args: {
		workspace: {
			...baseWorkspace,
			template_display_name: "",
		},
	},
	play: async ({ canvasElement, step }) => {
		const canvas = within(canvasElement);

		await step("activate hover trigger", async () => {
			await userEvent.hover(canvas.getByText(baseWorkspace.name));
			await waitFor(() =>
				expect(
					canvas.getByRole("presentation", { hidden: true }),
				).toHaveTextContent(MockTemplate.name),
			);
		});
	},
};
