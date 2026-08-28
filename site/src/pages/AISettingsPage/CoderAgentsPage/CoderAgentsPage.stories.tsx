import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { chatPersonalModelOverridesAdminSettings } from "#/api/queries/chats";
import { deploymentAgentTime } from "#/api/queries/deployment";
import type { Entitlements } from "#/api/typesGenerated";
import { DashboardContext } from "#/modules/dashboard/DashboardProvider";
import {
	MockAgentRuntimeHoursFeature,
	MockAppearanceConfig,
	MockBuildInfo,
	MockDefaultOrganization,
	MockEntitlements,
	MockUserOwner,
} from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import CoderAgentsPage from "./CoderAgentsPage";

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
} satisfies Entitlements["features"]["agent_runtime_hours"];
const entitlementsWithUnrelatedLicense: Entitlements = {
	...MockEntitlements,
	has_license: true,
	features: {
		...MockEntitlements.features,
		agent_runtime_hours: communityRuntimeFeature,
	},
};

const meta = {
	title: "pages/AISettingsPage/CoderAgentsPage/CoderAgentsPage",
	component: CoderAgentsPage,
	decorators: [
		withAuthProvider,
		(Story) => (
			<DashboardContext.Provider
				value={{
					entitlements: entitlementsWithUnrelatedLicense,
					experiments: [],
					appearance: MockAppearanceConfig,
					buildInfo: MockBuildInfo,
					organizations: [MockDefaultOrganization],
					showOrganizations: false,
					canViewOrganizationSettings: false,
				}}
			>
				<Story />
			</DashboardContext.Provider>
		),
	],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: { editDeploymentConfig: true },
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/coder-agents" },
			routing: [{ path: "*", useStoryElement: true }],
		}),
	},
} satisfies Meta<typeof CoderAgentsPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const CommunityUsageWithUnrelatedLicense: Story = {
	parameters: {
		queries: [
			{
				key: deploymentAgentTime().queryKey,
				data: { total_runtime_ms: actualMs },
			},
			{
				key: chatPersonalModelOverridesAdminSettings().queryKey,
				data: { allow_users: true },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByText("Agent hours used")).toBeVisible();
		await expect(canvas.getByText("10.3 hours")).toBeVisible();
		await expect(
			canvas.getByRole("link", {
				name: "Upgrade for unlimited concurrent chats",
			}),
		).toHaveAttribute("href", "/deployment/premium");
		await expect(canvas.getByText("5")).toBeVisible();
	},
};

export const MalformedUsage: Story = {
	parameters: {
		queries: [
			{
				key: deploymentAgentTime().queryKey,
				data: { total_runtime_ms: Number.NaN },
			},
			{
				key: chatPersonalModelOverridesAdminSettings().queryKey,
				data: { allow_users: true },
			},
		],
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByText("Agent Time usage is unavailable."),
		).toBeVisible();
		await expect(canvas.queryByText(/N\/A .*hours/)).not.toBeInTheDocument();
		await expect(
			canvas.getByRole("switch", { name: "Allow personal model overrides" }),
		).toBeVisible();
	},
};
