import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { PREMIUM_PAGE_PATH } from "#/components/Paywall/Paywall";
import {
	MockAgentRuntimeHoursFeature,
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	CoderAgentsPageView,
	type CoderAgentsPageViewProps,
} from "./CoderAgentsPageView";

const actualMs = (10 * 60 + 18) * 60_000;
const communityRuntimeFeature = {
	...MockAgentRuntimeHoursFeature,
	entitlement: "not_entitled",
	enabled: false,
	limit: undefined,
	soft_limit: undefined,
	hard_limit: undefined,
	actual: undefined,
	actual_ms: undefined,
	usage_period: undefined,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];
const licensedFiniteRuntimeFeature = {
	...MockAgentRuntimeHoursFeature,
	limit: 100,
	soft_limit: undefined,
	hard_limit: 120,
	actual: 10,
	actual_ms: undefined,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];
const licensedUnlimitedRuntimeFeature = {
	...MockAgentRuntimeHoursFeature,
	limit: undefined,
	soft_limit: undefined,
	hard_limit: undefined,
	actual: 10,
	actual_ms: undefined,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];
const licensedHardLimitRuntimeFeature = {
	...licensedFiniteRuntimeFeature,
	actual: 120,
	actual_ms: undefined,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];

const defaultArgs: CoderAgentsPageViewProps = {
	organization: MockDefaultOrganization,
	organizations: [MockDefaultOrganization, MockOrganization2],
	onSelectOrganization: fn(),
	requestedOrganizationDenied: false,
	isOrganizationAccessLoading: false,
	organizationSettings: <div>Organization override controls</div>,
	canEditDeploymentConfig: true,
	hasDeploymentLicense: false,
	hasAgentRuntimeLicense: false,
	agentRuntimeHoursFeature: communityRuntimeFeature,
	agentRuntimeTotalMs: actualMs,
	isAgentRuntimeUsageLoading: false,
	isAgentRuntimeUsageUnavailable: false,
	agentRuntimeUsageError: undefined,
	onRetryAgentRuntimeUsage: fn(),
	isRetryingAgentRuntimeUsage: false,
	adminOverridesData: { allow_users: true },
	onSaveAdminOverrides: fn(),
	isSavingAdminOverrides: false,
	isSaveAdminOverridesError: false,
	showAdvisorSettings: true,
	advisorConfigData: {
		enabled: true,
		max_uses_per_run: 5,
		max_output_tokens: 2048,
	},
	isAdvisorConfigLoading: false,
	isAdvisorConfigFetching: false,
	isAdvisorConfigLoadError: false,
	onSaveAdvisorConfig: fn(),
	isSavingAdvisorConfig: false,
	isSaveAdvisorConfigError: false,
	saveAdvisorConfigError: null,
	showVirtualDesktopSettings: false,
	computerUseProviderData: undefined,
	isLoadingComputerUseProvider: false,
	onSaveComputerUseProvider: fn(),
	isSavingComputerUseProvider: false,
	computerUseProviderSaveError: null,
};

const meta: Meta<typeof CoderAgentsPageView> = {
	title: "pages/AISettingsPage/CoderAgentsPage/CoderAgentsPageView",
	component: CoderAgentsPageView,
	args: defaultArgs,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/coder-agents" },
			routing: [{ path: "*", useStoryElement: true }],
		}),
	},
};
export default meta;
type Story = StoryObj<typeof CoderAgentsPageView>;

export const Default: Story = {
	args: { onSaveAdvisorConfig: fn() },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		const usage = canvas.getByRole("region", { name: "Coder Agents usage" });
		await expect(
			usage.compareDocumentPosition(
				canvas.getByRole("heading", { name: "Organization settings" }),
			) & Node.DOCUMENT_POSITION_FOLLOWING,
		).toBeTruthy();
		await expect(
			within(usage).getByRole("heading", { name: "Usage" }),
		).toBeVisible();
		await expect(canvas.getByText("10.3 hours")).toBeVisible();
		await expect(
			canvas.getByRole("link", {
				name: "Upgrade for unlimited concurrent agents",
			}),
		).toHaveAttribute("href", PREMIUM_PAGE_PATH);
		await userEvent.hover(
			canvas.getByRole("button", {
				name: "Agent hours used information",
			}),
		);
		await waitFor(async () => {
			await expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Total agent time used across all chats. Time from archived chats is excluded once deleted based on the conversation retention period.",
			);
		});
		await userEvent.keyboard("{Escape}");
		await waitFor(async () => {
			await expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();
		});
		await userEvent.hover(
			canvas.getByRole("button", {
				name: "Max concurrent agents information",
			}),
		);
		await waitFor(async () => {
			await expect(screen.getByRole("tooltip")).toHaveTextContent(
				"Number of agents that can run at the same time.",
			);
		});
		await expect(
			canvas.getByRole("heading", { name: "Organization settings" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("heading", { name: "Deployment settings" }),
		).toBeVisible();
		await expect(canvas.getByText("Advisor")).toBeVisible();
		const maxUses = canvas.getByRole("spinbutton", { name: "Uses / turn" });
		await userEvent.clear(maxUses);
		await userEvent.type(maxUses, "7");
		const save = canvas.getByRole("button", { name: "Save" });
		await waitFor(() => expect(save).toBeEnabled());
		await userEvent.click(save);
		await waitFor(() => {
			expect(args.onSaveAdvisorConfig).toHaveBeenCalledWith(
				{ max_uses_per_run: 7, max_output_tokens: 2048 },
				expect.anything(),
			);
		});
	},
};

export const WithoutAdvisor: Story = {
	args: { showAdvisorSettings: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Advisor")).not.toBeInTheDocument();
	},
};

export const UsageLoading: Story = {
	args: {
		hasAgentRuntimeLicense: undefined,
		agentRuntimeHoursFeature: undefined,
		isAgentRuntimeUsageLoading: true,
		agentRuntimeTotalMs: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: "Loading Agent Time usage" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("switch", { name: "Allow personal model overrides" }),
		).toBeVisible();
	},
};

export const UsageLoadError: Story = {
	args: {
		hasAgentRuntimeLicense: undefined,
		agentRuntimeHoursFeature: undefined,
		agentRuntimeUsageError: new Error("Failed to load Agent Time usage."),
		agentRuntimeTotalMs: undefined,
		onRetryAgentRuntimeUsage: fn(),
	},
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"Failed to load Agent Time usage.",
		);
		await expect(
			canvas.queryByText("Agent Time usage is unavailable."),
		).not.toBeInTheDocument();
		await userEvent.click(canvas.getByRole("button", { name: "Retry" }));
		await expect(args.onRetryAgentRuntimeUsage).toHaveBeenCalled();
		await expect(
			canvas.getByRole("switch", { name: "Allow personal model overrides" }),
		).toBeVisible();
	},
};

export const UsageUnavailable: Story = {
	args: {
		hasAgentRuntimeLicense: undefined,
		agentRuntimeHoursFeature: undefined,
		isAgentRuntimeUsageUnavailable: true,
		agentRuntimeTotalMs: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Agent Time usage is unavailable."),
		).toBeVisible();
		await expect(
			canvas.getByRole("switch", { name: "Allow personal model overrides" }),
		).toBeVisible();
	},
};

export const ZeroUsage: Story = {
	args: { agentRuntimeTotalMs: 0 },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("0.0 hours")).toBeVisible();
	},
};

export const LicensedFiniteAllocation: Story = {
	args: {
		hasAgentRuntimeLicense: true,
		agentRuntimeHoursFeature: licensedFiniteRuntimeFeature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("10.3 / 100 hours")).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "View license" }),
		).not.toBeInTheDocument();
	},
};

export const LicensedHardLimitReached: Story = {
	args: {
		hasAgentRuntimeLicense: true,
		agentRuntimeHoursFeature: licensedHardLimitRuntimeFeature,
		agentRuntimeTotalMs: 120 * 3_600_000,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const usage = within(
			canvas.getByRole("region", { name: "Coder Agents usage" }),
		);
		await expect(usage.getByText("120.0 / 100 hours")).toBeVisible();
		await expect(usage.getByText("5")).toBeVisible();
		await userEvent.hover(
			usage.getByRole("button", {
				name: "Max concurrent agents information",
			}),
		);
		await waitFor(async () => {
			await expect(screen.getByRole("tooltip")).toHaveTextContent(
				"You've reached your limit: concurrent chats are now capped at 5 (down from unlimited).",
			);
		});
	},
};

export const LicensedUnlimitedAllocation: Story = {
	args: {
		hasAgentRuntimeLicense: true,
		agentRuntimeHoursFeature: licensedUnlimitedRuntimeFeature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("10.3 / Unlimited hours")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "View usage documentation" }),
		).toBeVisible();
	},
};

export const SelectOrganization: Story = {
	args: { onSelectOrganization: fn() },
	play: async ({ canvasElement, args }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			canvas.getByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		);
		await userEvent.click(
			await screen.findByRole("option", {
				name: new RegExp(MockOrganization2.display_name, "i"),
			}),
		);
		await expect(args.onSelectOrganization).toHaveBeenCalledWith(
			MockOrganization2,
		);
	},
};

export const OrganizationOnly: Story = {
	args: { canEditDeploymentConfig: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Organization settings" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: "Deployment settings" }),
		).not.toBeInTheDocument();
	},
};

export const OrganizationDiscoveryLoading: Story = {
	args: { isOrganizationAccessLoading: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("status", { name: "Loading organization settings" }),
		).toBeVisible();
		await expect(
			canvas.queryByRole("heading", { name: "Organization settings" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByText("Organization override controls"),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("heading", { name: "Deployment settings" }),
		).toBeVisible();
	},
};

export const DeploymentOnly: Story = {
	args: {
		organization: undefined,
		organizations: [],
		organizationSettings: undefined,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("heading", { name: "Organization settings" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("heading", { name: "Deployment settings" }),
		).toBeVisible();
	},
};

export const SingleOrganization: Story = {
	args: { organizations: [MockDefaultOrganization] },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.queryByRole("button", {
				name: new RegExp(MockDefaultOrganization.display_name, "i"),
			}),
		).not.toBeInTheDocument();
	},
};

export const InaccessibleRequestedOrganization: Story = {
	args: { requestedOrganizationDenied: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByRole("alert")).toHaveTextContent(
			"requested organization is not available",
		);
		await expect(
			canvas.getByText("Organization override controls"),
		).toBeVisible();
	},
};

export const IndependentErrors: Story = {
	args: {
		organizationAccessError: new Error("Failed to load another organization"),
		adminOverridesError: new Error("Failed to load deployment setting"),
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Failed to load another organization"),
		).toBeVisible();
		await expect(
			canvas.getByText("Failed to load deployment setting"),
		).toBeVisible();
		await expect(
			canvas.getByText("Organization override controls"),
		).toBeVisible();
	},
};

export const Mobile: Story = {
	parameters: { viewport: { defaultViewport: "mobile1" } },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Organization settings" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("heading", { name: "Deployment settings" }),
		).toBeVisible();
	},
};
