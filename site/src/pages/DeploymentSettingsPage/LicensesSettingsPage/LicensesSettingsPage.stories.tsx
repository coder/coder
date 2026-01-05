import { MockLicenseResponse } from "testHelpers/entities";
import type { Meta, StoryObj } from "@storybook/react-vite";
import LicensesSettingsPage from "./LicensesSettingsPage";

const meta: Meta<typeof LicensesSettingsPage> = {
	title: "pages/DeploymentSettingsPage/LicensesSettingsPage",
	component: LicensesSettingsPage,
	parameters: {
		queries: [
			{ key: ["licenses"], data: MockLicenseResponse },
			{ key: ["insights", "userStatusCounts"], data: { active: [] } },
		],
	},
};

export default meta;
type Story = StoryObj<typeof LicensesSettingsPage>;

export const WithoutUserLimitFeature: Story = {
	parameters: {
		queries: [
			{ key: ["entitlements"], data: { features: {} } },
			{ key: ["licenses"], data: MockLicenseResponse },
			{ key: ["insights", "userStatusCounts"], data: { active: [] } },
		],
	},
};
