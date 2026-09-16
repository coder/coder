import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { SerpentGroup } from "#/api/typesGenerated";
import { docs } from "#/utils/docs";
import { NetworkSettingsPageView } from "./NetworkSettingsPageView";

const group: SerpentGroup = {
	name: "Networking",
	description: "",
};

const meta: Meta<typeof NetworkSettingsPageView> = {
	title: "pages/DeploymentSettingsPage/NetworkSettingsPageView",
	component: NetworkSettingsPageView,
	args: {
		options: [
			{
				name: "DERP Server Enable",
				description:
					"Whether to enable or disable the embedded DERP relay server.",
				value: true,
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "DERP Server Region Name",
				description: "Region name that for the embedded DERP server.",
				value: "aws-east",
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "DERP Server STUN Addresses",
				description:
					"Addresses for STUN servers to establish P2P connections. Set empty to disable P2P connections.",
				value: ["stun.l.google.com:19302", "stun.l.google.com:19301"],
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "DERP Config URL",
				description:
					"URL to fetch a DERP mapping on startup. See: https://tailscale.com/kb/1118/custom-derp-servers/",
				value: "https://coder.com",
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "Wildcard Access URL",
				description: "",
				value: "https://coder.com",
				group,
				flag: "derp",
				flag_shorthand: "d",
				hidden: false,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof NetworkSettingsPageView>;

export const Page: Story = {
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const docsLinks = canvas.getAllByRole("link", { name: /View docs/ });
		await expect(docsLinks).toHaveLength(2);
		await expect(docsLinks[0]).toHaveAttribute(
			"href",
			docs("/admin/networking"),
		);
		await expect(docsLinks[1]).toHaveAttribute(
			"href",
			docs("/admin/networking/port-forwarding"),
		);
	},
};
