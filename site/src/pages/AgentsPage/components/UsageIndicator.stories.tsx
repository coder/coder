import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { meAISpendKey } from "#/api/queries/users";
import { getWorkspaceQuotaQueryKey } from "#/api/queries/workspaceQuota";
import { workspacesKey } from "#/api/queries/workspaces";
import type {
	UserAISpendStatus,
	WorkspaceQuota,
	WorkspacesResponse,
} from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockPermissions,
	MockUserOwner,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import { UsageIndicator } from "./UsageIndicator";

const withAISpend = (status: UserAISpendStatus) => (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(meAISpendKey, status);
	return <Story />;
};

const withWorkspaceQuota = (quota: WorkspaceQuota) => (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(
		getWorkspaceQuotaQueryKey(
			MockDefaultOrganization.name,
			MockUserOwner.username,
		),
		quota,
	);
	return <Story />;
};

const withWorkspaceCount = (count: number) => (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(workspacesKey(userWorkspacesRequest), {
		workspaces: [],
		count,
	} satisfies WorkspacesResponse);
	return <Story />;
};

const withUnavailableWorkspaceCount = (Story: FC) => {
	const queryClient = useQueryClient();
	queryClient.setQueryData(workspacesKey(userWorkspacesRequest), {
		workspaces: [],
		count: -1,
	} satisfies WorkspacesResponse);
	return <Story />;
};

// Mirrors the sidebar footer wrapper: a fixed-width container with
// container-type set so the trigger inside reacts to the wrapper's width
// instead of the viewport's.
const withUsageIndicatorFrame = (
	widthClassName = "w-[320px]",
	frameTestId?: string,
): Decorator => {
	return (Story) => (
		<div
			data-testid={frameTestId}
			className={`flex h-12 min-w-0 items-stretch justify-end rounded-md bg-surface-secondary @container ${widthClassName}`}
		>
			<Story />
		</div>
	);
};

const openUsageMenu = async (canvasElement: HTMLElement) => {
	const canvas = within(canvasElement);
	await userEvent.click(canvas.getByRole("button"));
};

const aiSpendStatus = (
	overrides: Partial<UserAISpendStatus> = {},
): UserAISpendStatus => ({
	user_id: MockUserOwner.id,
	effective_group_id: "group-1",
	effective_budget: { spend_limit_micros: 50_000_000, limit_source: "group" },
	current_spend_micros: 12_500_000,
	period_start: "2026-07-01T00:00:00Z",
	period_end: "2026-08-01T00:00:00Z",
	...overrides,
});

const noBudgetStatus = aiSpendStatus({
	effective_group_id: null,
	effective_budget: null,
	current_spend_micros: 0,
});

const userWorkspacesRequest = {
	q: `owner:me organization:${MockDefaultOrganization.name}`,
	limit: 0,
};
const noWorkspaceQuota = {
	credits_consumed: 0,
	budget: 0,
} satisfies WorkspaceQuota;
const defaultWorkspaceQuota = {
	credits_consumed: 30,
	budget: 100,
} satisfies WorkspaceQuota;

const meta: Meta<typeof UsageIndicator> = {
	title: "pages/AgentsPage/UsageIndicator",
	component: UsageIndicator,
	decorators: [
		withAuthProvider,
		withDashboardProvider,
		withUsageIndicatorFrame(),
	],
	parameters: {
		user: MockUserOwner,
		permissions: MockPermissions,
		features: ["aibridge"],
	},
};

export default meta;
type Story = StoryObj<typeof UsageIndicator>;

export const LowUsage: Story = {
	decorators: [
		withAISpend(aiSpendStatus()),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const MediumUsage: Story = {
	decorators: [
		withAISpend(
			aiSpendStatus({
				effective_budget: {
					spend_limit_micros: 20_000_000,
					limit_source: "group",
				},
				current_spend_micros: 16_000_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const HighUsage: Story = {
	decorators: [
		withAISpend(
			aiSpendStatus({
				effective_budget: {
					spend_limit_micros: 10_000_000,
					limit_source: "group",
				},
				current_spend_micros: 9_500_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const LimitExceeded: Story = {
	decorators: [
		withAISpend(
			aiSpendStatus({
				effective_budget: {
					spend_limit_micros: 30_000_000,
					limit_source: "group",
				},
				current_spend_micros: 32_000_000,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const WorkspaceQuotaOnly: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(
			canvas.getByRole("progressbar", { name: "Workspace quota usage" }),
		).toBeVisible();
		await openUsageMenu(canvasElement);
	},
};

export const UsageAndWorkspaceQuota: Story = {
	decorators: [
		withAISpend(aiSpendStatus()),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const progressBars = canvas.getAllByRole("progressbar");

		expect(canvas.getByRole("button", { name: "Usage" })).toBeVisible();
		expect(progressBars.map((bar) => bar.getAttribute("aria-label"))).toEqual([
			"AI spend usage",
			"Workspace quota usage",
		]);

		await openUsageMenu(canvasElement);
		const menu = within(await within(document.body).findByRole("menu"));
		await waitFor(() => {
			expect(menu.getByText("$12.50 of $50.00 used")).toBeVisible();
			expect(menu.getByText("July 1 - August 1, 2026")).toBeVisible();
		});
	},
};

export const TriggerTiny: Story = {
	decorators: [
		withUsageIndicatorFrame("w-[240px]", "usage-indicator-frame"),
		withAISpend(aiSpendStatus()),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
};

export const WorkspaceQuotaUnused: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota({
			credits_consumed: 0,
			budget: 100,
		}),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		expect(canvas.queryByRole("button")).not.toBeInTheDocument();
	},
};

export const WorkspaceQuotaWithoutBudget: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota({
			credits_consumed: 20,
			budget: 0,
		}),
		withWorkspaceCount(1),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const progressbar = canvas.getByRole("progressbar", {
			name: "Workspace quota usage",
		});

		expect(progressbar).toHaveAttribute("aria-valuenow", "100");

		await openUsageMenu(canvasElement);
		expect(within(document.body).getByText("100%")).toBeInTheDocument();
		expect(
			within(document.body).getByText("1 workspace using 20 of 0 credits"),
		).toBeInTheDocument();
	},
};

export const WorkspaceQuotaExceeded: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota({
			credits_consumed: 125,
			budget: 100,
		}),
		withWorkspaceCount(7),
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const WorkspaceQuotaWithoutWorkspaceCount: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withUnavailableWorkspaceCount,
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
	},
};

export const NotLimited: Story = {
	decorators: [
		withAISpend(noBudgetStatus),
		withWorkspaceQuota(noWorkspaceQuota),
	],
};

export const ZeroBudget: Story = {
	decorators: [
		withAISpend(
			aiSpendStatus({
				effective_budget: { spend_limit_micros: 0, limit_source: "group" },
				current_spend_micros: 0,
			}),
		),
		withWorkspaceQuota(noWorkspaceQuota),
	],
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const trigger = await canvas.findByRole("button");
		await userEvent.click(trigger);
		const menu = within(
			await within(canvasElement.ownerDocument.body).findByRole("menu"),
		);
		await waitFor(() => {
			expect(menu.getByText(/limit exceeded/)).toBeVisible();
		});
	},
};

export const GatewayUnavailable: Story = {
	parameters: { features: [], experiments: [] },
	decorators: [
		withAISpend(aiSpendStatus()),
		withWorkspaceQuota(defaultWorkspaceQuota),
		withWorkspaceCount(3),
	],
	play: async ({ canvasElement }) => {
		await openUsageMenu(canvasElement);
		const progressBars = within(canvasElement.ownerDocument.body).getAllByRole(
			"progressbar",
		);

		expect(progressBars.map((bar) => bar.getAttribute("aria-label"))).toEqual([
			"Workspace quota usage",
		]);
	},
};
