import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { SerpentGroup, SerpentOption } from "#/api/typesGenerated";
import { docs } from "#/utils/docs";
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
		isBrowserOnlyEntitled: true,
		featureBrowserOnlyEnabled: true,
	},
};

export default meta;
type Story = StoryObj<typeof SecuritySettingsPageView>;

export const Page: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const docsLinks = canvas.getAllByRole("link", { name: /View docs/ });
		await expect(docsLinks).toHaveLength(2);
		await expect(docsLinks[0]).toHaveAttribute("href", docs("/admin/security"));
		await expect(docsLinks[1]).toHaveAttribute(
			"href",
			docs("/admin/networking#browser-only-connections"),
		);
		await expect(
			canvas.getByRole("heading", {
				name: /Browser-Only Connections Enabled/,
			}),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: /Start trial for free/ }),
		).not.toBeInTheDocument();
	},
};

export const EntitledAndTurnedOff: Story = {
	args: {
		isBrowserOnlyEntitled: true,
		featureBrowserOnlyEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", {
				name: /Browser-Only Connections Disabled/,
			}),
		).toBeInTheDocument();
		await expect(
			canvas.queryByRole("link", { name: /Start trial for free/ }),
		).not.toBeInTheDocument();
	},
};

export const NotEntitled: Story = {
	args: {
		isBrowserOnlyEntitled: false,
		featureBrowserOnlyEnabled: false,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("heading", {
				name: /Browser-Only Connections Disabled/,
			}),
		).toBeInTheDocument();
		await expect(
			canvas.getByRole("link", { name: /Start trial for free/ }),
		).toBeInTheDocument();
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
