import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { SerpentGroup } from "#/api/typesGenerated";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const group: SerpentGroup = {
	name: "Introspection",
	description: "",
};

const meta: Meta<typeof ObservabilitySettingsPageView> = {
	title: "pages/DeploymentSettingsPage/ObservabilitySettingsPageView",
	component: ObservabilitySettingsPageView,
	args: {
		options: [
			{
				name: "Verbose",
				value: true,
				group,
				flag: "verbose",
				flag_shorthand: "v",
				hidden: false,
			},
			{
				name: "Human Log Location",
				description: "Output human-readable logs to a given file.",
				value: "/dev/stderr",
				flag: "log-human",
				hidden: false,
			},
			{
				name: "Stackdriver Log Location",
				description: "Output Stackdriver compatible logs to a given file.",
				value: "",
				flag: "log-stackdriver",
				hidden: false,
			},
			{
				name: "Prometheus Enable",
				description:
					"Serve prometheus metrics on the address defined by prometheus address.",
				value: true,
				group: { ...group },
				flag: "prometheus-enable",
				hidden: false,
			},
		],
		featureAuditLogEnabled: true,
		isPremium: false,
	},
};

export default meta;
type Story = StoryObj<typeof ObservabilitySettingsPageView>;

export const Page: Story = {};

export const OSS: Story = {
	args: { featureAuditLogEnabled: false, isPremium: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		const alert = canvas.getByRole("alert");
		await expect(alert).toBeVisible();
		await expect(within(alert).getByText("Premium")).toBeVisible();
		await expect(
			within(alert).getByRole("link", {
				name: "Read the Audit Logs documentation",
			}),
		).toBeVisible();
		await expect(canvas.queryByText("Enterprise")).not.toBeInTheDocument();
	},
};

export const Premium: Story = {
	args: { featureAuditLogEnabled: true, isPremium: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
		await expect(canvas.getByText("Premium")).toBeVisible();
	},
};

export const EnterpriseAuditLogs: Story = {
	args: { featureAuditLogEnabled: true, isPremium: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
		await expect(canvas.getByText("Enterprise")).toBeVisible();
	},
};
