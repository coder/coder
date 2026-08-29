import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, screen, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	MockDefaultOrganization,
	MockOrganization2,
} from "#/testHelpers/entities";
import {
	CoderAgentsPageView,
	type CoderAgentsPageViewProps,
} from "./CoderAgentsPageView";

const defaultArgs: CoderAgentsPageViewProps = {
	organization: MockDefaultOrganization,
	organizations: [MockDefaultOrganization, MockOrganization2],
	onSelectOrganization: fn(),
	requestedOrganizationDenied: false,
	isOrganizationAccessLoading: false,
	organizationSettings: <div>Organization override controls</div>,
	canEditDeploymentConfig: true,
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
