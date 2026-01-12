import type { Meta, StoryObj } from "@storybook/react-vite";
import type { SerpentGroup, SerpentOption } from "api/typesGenerated";
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
				flag: "ssh-keygen-algorithm",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "Secure Auth Cookie",
				description: "something",
				value: "1234",
				flag: "secure-auth-cookie",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "Disable Owner Workspace Access",
				description: "something",
				value: false,
				flag: "disable-owner-workspace-access",
				flag_shorthand: "d",
				hidden: false,
			},
			{
				name: "TLS Version",
				description: "something",
				value: ["something"],
				group: { ...group, name: "TLS" },
				flag: "tls-enable",
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

export const NoTLS: Story = {
	args: {
		options: [
			{
				name: "SSH Keygen Algorithm",
				value: "1234",
				flag: "ssh-keygen-algorithm",
			} as SerpentOption,
			{
				name: "Disable Owner Workspace Access",
				value: false,
				flag: "disable-owner-workspace-access",
			} as SerpentOption,
			{
				name: "Secure Auth Cookie",
				value: "1234",
				flag: "secure-auth-cookie",
			} as SerpentOption,
		],
	},
};
