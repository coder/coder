import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import {
	CoderAgentsPageView,
	type CoderAgentsPageViewProps,
} from "./CoderAgentsPageView";

const defaultArgs: CoderAgentsPageViewProps = {
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
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getAllByRole("link", { name: "Defaults & overrides" })[0],
		).toBeVisible();
		await expect(canvas.getByText("Advisor")).toBeVisible();
	},
};

export const WithoutAdvisor: Story = {
	args: { showAdvisorSettings: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.queryByText("Advisor")).not.toBeInTheDocument();
	},
};
