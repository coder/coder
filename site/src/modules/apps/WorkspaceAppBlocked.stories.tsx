import type { Meta, StoryObj } from "@storybook/react-vite";
import { MockUserOwner } from "#/testHelpers/entities";
import { withAuthProvider } from "#/testHelpers/storybook";
import type { FrameBlock } from "./useFrameBlock";
import { WorkspaceAppBlocked } from "./WorkspaceAppBlocked";

const mockCoderPolicyBlock: FrameBlock = {
	src: "https://code-server--main--workspace--alice.apps.example.com:8443/",
	origin: "https://code-server--main--workspace--alice.apps.example.com:8443",
	byCoderPolicy: true,
};

const mockProxyPolicyBlock: FrameBlock = {
	...mockCoderPolicyBlock,
	byCoderPolicy: false,
};

const meta: Meta<typeof WorkspaceAppBlocked> = {
	title: "modules/apps/WorkspaceAppBlocked",
	component: WorkspaceAppBlocked,
	decorators: [withAuthProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
	},
	args: {
		reason: { kind: "missing-wildcard" },
	},
};

export default meta;
type Story = StoryObj<typeof meta>;

export const MissingWildcard: Story = {};

export const MissingWildcardAdmin: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: true,
		},
	},
};

export const CoderPolicy: Story = {
	args: {
		reason: { kind: "csp", ...mockCoderPolicyBlock },
	},
};

export const ProxyPolicy: Story = {
	args: {
		reason: { kind: "csp", ...mockProxyPolicyBlock },
	},
};

export const ProxyPolicyAdmin: Story = {
	args: {
		reason: { kind: "csp", ...mockProxyPolicyBlock },
	},
	parameters: {
		permissions: {
			editDeploymentConfig: true,
		},
	},
};
