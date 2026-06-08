import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { MockBuildInfo, MockProvisioner } from "#/testHelpers/entities";
import { ProvisionerVersion } from "./ProvisionerVersion";

const meta: Meta<typeof ProvisionerVersion> = {
	title: "pages/OrganizationProvisionersPage/ProvisionerVersion",
	component: ProvisionerVersion,
	args: {
		buildVersion: MockBuildInfo.version,
		buildAPIVersion: MockBuildInfo.provisioner_api_version,
		provisionerVersion: MockProvisioner.version,
		provisionerAPIVersion: MockProvisioner.api_version,
	},
};

export default meta;
type Story = StoryObj<typeof ProvisionerVersion>;

export const UpToDate: Story = {};

// Provisioner build version differs from the server, but the provisioner API
// version is in the server's supported range. This is the case Kayla hit:
// a v2.33 provisioner against a v2.32 server, both on the same provisioner
// API. The UI should not warn.
export const Compatible: Story = {
	args: {
		provisionerVersion: "v2.32.5",
		buildVersion: "v2.33.0",
		provisionerAPIVersion: "1.17",
		buildAPIVersion: "1.18",
	},
};

// Provisioner is newer than the server at the API level. The server is the
// component that needs upgrading.
export const ServerAhead: Story = {
	args: {
		provisionerVersion: "v2.34.0",
		buildVersion: "v2.33.0",
		provisionerAPIVersion: "1.19",
		buildAPIVersion: "1.18",
	},
};

// Provisioner reports an older major API version. The server may or may not
// still accept it via backward-compat, but the UI cannot confirm.
export const Outdated: Story = {
	args: {
		provisionerVersion: "v2.10.0",
		buildVersion: MockBuildInfo.version,
		provisionerAPIVersion: "0.9",
		buildAPIVersion: MockBuildInfo.provisioner_api_version,
	},
};

// Provisioner did not report an API version (older daemon or unexpected data),
// so we cannot compare meaningfully. Fall back to a generic mismatch label.
export const Unknown: Story = {
	args: {
		provisionerVersion: "0.0.0",
		buildVersion: MockBuildInfo.version,
		provisionerAPIVersion: "",
		buildAPIVersion: MockBuildInfo.provisioner_api_version,
	},
};

export const OnFocus: Story = {
	args: {
		provisionerVersion: "v2.10.0",
		buildVersion: MockBuildInfo.version,
		provisionerAPIVersion: "0.9",
		buildAPIVersion: MockBuildInfo.provisioner_api_version,
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const version = canvas.getByText(/outdated/i);
		await userEvent.tab();
		expect(version).toHaveFocus();
	},
};
