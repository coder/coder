import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { SerpentGroup } from "#/api/typesGenerated";
import { MockPermissions } from "#/testHelpers/entities";
import { ObservabilitySettingsPageView } from "./ObservabilitySettingsPageView";

const introspectionGroup: SerpentGroup = {
	name: "Introspection",
	description: "",
};

const retentionGroup: SerpentGroup = {
	name: "Retention",
	description: "",
};

const meta: Meta<typeof ObservabilitySettingsPageView> = {
	title: "pages/DeploymentSettingsPage/ObservabilitySettingsPageView",
	component: ObservabilitySettingsPageView,
	args: {
		options: [
			{
				name: "Audit Logs Retention",
				description:
					"How long audit log entries are retained. Set to 0 to disable (keep indefinitely).",
				value: 0,
				group: retentionGroup,
				flag: "audit-logs-retention",
				env: "CODER_AUDIT_LOGS_RETENTION",
				yaml: "audit_logs",
				hidden: false,
			},
			{
				name: "Verbose",
				value: true,
				group: introspectionGroup,
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
				group: { ...introspectionGroup },
				flag: "prometheus-enable",
				hidden: false,
			},
		],
		featureAuditLogEnabled: true,
		canViewPremium: MockPermissions.viewAllLicenses,
	},
};

export default meta;
type Story = StoryObj<typeof ObservabilitySettingsPageView>;

export const Page: Story = {};

export const OSS: Story = {
	args: { featureAuditLogEnabled: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(canvas.getByText("Audit Logging")).toBeVisible();
		await expect(
			canvas.getByRole("link", { name: "Start trial for free" }),
		).toHaveAttribute("href", "/deployment/premium");
		await expect(
			canvas.getByRole("link", { name: "Learn more about premium" }),
		).toBeVisible();
		await expect(
			canvas.queryByText("Audit Logs Retention"),
		).not.toBeInTheDocument();
	},
};

export const OSSWithoutLicenseAccess: Story = {
	args: { featureAuditLogEnabled: false, canViewPremium: false },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.getByText(/contact your deployment administrator/i),
		).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
	},
};

export const Entitled: Story = {
	args: { featureAuditLogEnabled: true },
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);

		await expect(
			canvas.queryByRole("link", { name: "Start trial for free" }),
		).not.toBeInTheDocument();
		await expect(canvas.getByText("Audit Logs Retention")).toBeVisible();
	},
};
