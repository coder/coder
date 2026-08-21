import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { SerpentGroup, SerpentOption } from "#/api/typesGenerated";
import { SecuritySettingsPageView } from "./SecuritySettingsPageView";

const group: SerpentGroup = {
	name: "Networking",
	description: "",
};

const meta: Meta<typeof SecuritySettingsPageView> = {
	title: "pages/DeploymentSettingsPage/SecuritySettingsPageView",
	component: SecuritySettingsPageView,
	args: {
		options: [
			{
				name: "SSH Keygen Algorithm",
				description: "something",
				value: "1234",
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "Secure Auth Cookie",
				description: "something",
				value: "1234",
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "Disable Owner Workspace Access",
				description: "something",
				value: false,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "TLS Version",
				description: "something",
				value: ["something"],
				group: { ...group, name: "TLS" },
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
		],
		featureBrowserOnlyEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof SecuritySettingsPageView>;

export const Page: Story = {};

export const BrowserOnlyPaywall: Story = {
	args: {
		featureBrowserOnlyEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText("Restrict access to web-based connections"),
		).toBeVisible();
		await expect(
			canvas.getByText("Block SSH and port-forward entirely"),
		).toBeVisible();
		await expect(
			canvas.getByText("Enforce browser-only compliance policies"),
		).toBeVisible();
		// The generic premium bullets must not leak back in.
		await expect(
			canvas.queryByText("24x7 global support with SLA"),
		).not.toBeInTheDocument();
	},
};

export const NoTLS = {
	args: {
		options: [
			{
				name: "SSH Keygen Algorithm",
				value: "1234",
			} as SerpentOption,
			{
				name: "Disable Owner Workspace Access",
				value: false,
			} as SerpentOption,
			{
				name: "Secure Auth Cookie",
				value: "1234",
			} as SerpentOption,
		],
	},
};
