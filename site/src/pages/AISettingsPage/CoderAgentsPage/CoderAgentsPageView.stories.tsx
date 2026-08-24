import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { MockAgentRuntimeHoursFeature } from "#/testHelpers/entities";
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
	actual: 10,
	actual_ms: actualMs,
	usage_period: undefined,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];
const licensedFiniteRuntimeFeature = {
	...MockAgentRuntimeHoursFeature,
	limit: 100,
	soft_limit: undefined,
	hard_limit: 120,
	actual: 10,
	actual_ms: actualMs,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];
const licensedUnlimitedRuntimeFeature = {
	...MockAgentRuntimeHoursFeature,
	limit: undefined,
	soft_limit: undefined,
	hard_limit: undefined,
	actual: 10,
	actual_ms: actualMs,
} satisfies CoderAgentsPageViewProps["agentRuntimeHoursFeature"];

const defaultArgs: CoderAgentsPageViewProps = {
	hasAgentRuntimeLicense: false,
	agentRuntimeHoursFeature: communityRuntimeFeature,
	isAgentRuntimeUsageLoading: false,
	isAgentRuntimeUsageUnavailable: false,
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
		await expect(canvas.getByRole("heading", { name: "Usage" })).toBeVisible();
		await expect(canvas.getByText("10.3 hours")).toBeVisible();
		await expect(canvas.getByRole("link", { name: "Upgrade" })).toHaveAttribute(
			"href",
			"/deployment/premium",
		);
		await expect(
			canvas.getAllByRole("link", { name: "Defaults & overrides" })[0],
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

export const UsageUnavailable: Story = {
	args: {
		hasAgentRuntimeLicense: undefined,
		agentRuntimeHoursFeature: undefined,
		isAgentRuntimeUsageUnavailable: true,
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

export const LicensedFiniteAllocation: Story = {
	args: {
		hasAgentRuntimeLicense: true,
		agentRuntimeHoursFeature: licensedFiniteRuntimeFeature,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("10.3 / 100 hours")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Manage license" }),
		).toHaveAttribute("href", "/deployment/licenses");
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
