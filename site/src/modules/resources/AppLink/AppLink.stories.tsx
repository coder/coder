import type { Meta, StoryObj } from "@storybook/react";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import {
	MockPrimaryWorkspaceProxy,
	MockProxyLatencies,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceProxies,
} from "testHelpers/entities";
import { withGlobalSnackbar } from "testHelpers/storybook";
import { AppLink } from "./AppLink";

const meta: Meta<typeof AppLink> = {
	title: "modules/resources/AppLink",
	component: AppLink,
	decorators: [
		(Story) => (
			<ProxyContext.Provider
				value={{
					proxyLatencies: MockProxyLatencies,
					proxy: {
						...getPreferredProxy(
							MockWorkspaceProxies,
							MockPrimaryWorkspaceProxy,
						),
						preferredWildcardHostname: "*.super_proxy.tld",
					},
					proxies: MockWorkspaceProxies,
					isLoading: false,
					isFetched: true,
					setProxy: () => {
						return;
					},
					clearProxy: () => {
						return;
					},
					refetchProxyLatencies: (): Date => {
						return new Date();
					},
				}}
			>
				<Story />
			</ProxyContext.Provider>
		),
	],
};

export default meta;
type Story = StoryObj<typeof AppLink>;

export const WithIcon: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			icon: "/icon/code.svg",
			sharing_level: "owner",
			health: "healthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const WithNonSquaredIcon: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			icon: "/icon/windsurf.svg",
			sharing_level: "owner",
			health: "healthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const ExternalApp: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
		},
		agent: MockWorkspaceAgent,
	},
};

export const ExternalAppNotInstalled: Story = {
	decorators: [withGlobalSnackbar],
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			external: true,
			url: "foobar-foobaz://open-me",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelOwner: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "owner",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelAuthenticated: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "authenticated",
		},
		agent: MockWorkspaceAgent,
	},
};

export const SharingLevelPublic: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "public",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthDisabled: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			sharing_level: "owner",
			health: "disabled",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthInitializing: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			health: "initializing",
		},
		agent: MockWorkspaceAgent,
	},
};

export const HealthUnhealthy: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			health: "unhealthy",
		},
		agent: MockWorkspaceAgent,
	},
};

export const InternalApp: Story = {
	args: {
		workspace: MockWorkspace,
		app: {
			...MockWorkspaceApp,
			display_name: "Check my URL",
			subdomain: true,
			subdomain_name: "slug--agent_name--workspace_name--username",
		},
		agent: MockWorkspaceAgent,
	},
};

export const BlockingStartupScriptRunning: Story = {
	args: {
		workspace: MockWorkspace,
		app: MockWorkspaceApp,
		agent: {
			...MockWorkspaceAgent,
			lifecycle_state: "starting",
			startup_script_behavior: "blocking",
		},
	},
};
