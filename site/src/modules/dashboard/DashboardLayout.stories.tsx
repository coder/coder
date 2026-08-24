import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
	reactRouterOutlet,
	reactRouterParameters,
} from "storybook-addon-remix-react-router";
import { buildInfoKey } from "#/api/queries/buildInfo";
import { chatModels } from "#/api/queries/chats";
import { deploymentStatsQueryKey } from "#/api/queries/deployment";
import { organizationsPermissions } from "#/api/queries/organizations";
import { updateCheckQueryKey } from "#/api/queries/updateCheck";
import type { UpdateCheckResponse } from "#/api/typesGenerated";
import {
	MockChatModel,
	MockChatModelProviderDescriptor,
} from "#/testHelpers/chatModels";
import {
	MockBuildInfo,
	MockDefaultOrganization,
	MockDeploymentStats,
	MockNoOrganizationPermissions,
	MockNoPermissions,
	MockPermissions,
	MockUpdateCheck,
	MockUserMember,
	MockUserOwner,
} from "#/testHelpers/entities";
import { pixelWithTablet } from "#/testHelpers/pixel";
import {
	withAuthProvider,
	withDashboardProvider,
	withProxyProvider,
} from "#/testHelpers/storybook";
import { DashboardFullPage, DashboardLayout } from "./DashboardLayout";

const outdatedUpdateCheck: UpdateCheckResponse = {
	...MockUpdateCheck,
	current: false,
	version: "v0.12.9",
	url: "https://github.com/coder/coder/releases/tag/v0.12.9",
};

const pageContent = (
	<DashboardFullPage className="p-6">
		<h1>Workspaces</h1>
		<p>Page content rendered in the dashboard outlet.</p>
	</DashboardFullPage>
);

const modelSettingsRouter = reactRouterParameters({
	location: { path: "/" },
	routing: [
		{ path: "/", useStoryElement: true },
		{
			path: "/ai/settings/models",
			element: <h1>Organization models</h1>,
		},
	],
});

const meta: Meta<typeof DashboardLayout> = {
	title: "modules/dashboard/DashboardLayout",
	component: DashboardLayout,
	decorators: [withAuthProvider, withDashboardProvider, withProxyProvider()],
	parameters: {
		layout: "fullscreen",
		pixel: { matrix: pixelWithTablet },
		user: MockUserOwner,
		permissions: MockPermissions,
		reactRouter: reactRouterParameters({
			location: { path: "/" },
			routing: reactRouterOutlet({ path: "/" }, pageContent),
		}),
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: MockUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
		],
	},
};

export default meta;
type Story = StoryObj<typeof DashboardLayout>;

export const Default: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Workspaces" }),
		).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Skip to main content" }),
		).toBeInTheDocument();
	},
};

export const ForMember: Story = {
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", { name: "Workspaces" }),
		).toBeVisible();
		await expect(
			screen.queryByTestId("update-check-notice"),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: "Admin settings" }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: "Models" }),
		).not.toBeInTheDocument();
	},
};

export const CustomOrganizationRoleCanOpenModels: Story = {
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
		reactRouter: modelSettingsRouter,
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: MockUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
			{
				key: organizationsPermissions([MockDefaultOrganization.id]).queryKey,
				data: {
					[MockDefaultOrganization.id]: {
						...MockNoOrganizationPermissions,
						viewChatModelConfigs: true,
					},
				},
			},
		],
	},
	play: openModels,
};

export const ACLReadableMemberCanOpenModels: Story = {
	parameters: {
		user: MockUserMember,
		permissions: MockNoPermissions,
		reactRouter: modelSettingsRouter,
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: MockUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
			{
				key: chatModels(MockDefaultOrganization.id).queryKey,
				data: {
					models: [
						{
							...MockChatModel,
							organization_id: MockDefaultOrganization.id,
						},
					],
					providers: [MockChatModelProviderDescriptor],
				},
			},
		],
	},
	play: openModels,
};

export const UpdateAvailable: Story = {
	parameters: {
		queries: [
			{ key: buildInfoKey, data: MockBuildInfo },
			{ key: updateCheckQueryKey, data: outdatedUpdateCheck },
			{ key: deploymentStatsQueryKey, data: MockDeploymentStats },
		],
	},
	beforeEach: () => {
		localStorage.removeItem("dismissedVersion");
	},
	play: async () => {
		const notice = await screen.findByTestId("update-check-notice");
		await expect(notice).toBeVisible();
		await expect(
			screen.getByText(/Coder v0\.12\.9 is now available/),
		).toBeVisible();
	},
};

export const SkipToMainContent: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const skipLink = canvas.getByRole("link", {
			name: "Skip to main content",
		});
		const main = canvas.getByRole("main");

		await userEvent.click(skipLink);
		await expect(main).toHaveFocus();
	},
};

async function openModels({ canvasElement }: { canvasElement: HTMLElement }) {
	const user = userEvent.setup();
	const canvas = within(canvasElement);
	await expect(
		canvas.queryByRole("button", { name: "Admin settings" }),
	).not.toBeInTheDocument();
	await user.click(canvas.getByRole("link", { name: "Models" }));
	await expect(
		await canvas.findByRole("heading", { name: "Organization models" }),
	).toBeInTheDocument();
}
