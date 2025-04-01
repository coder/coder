import type { Meta, StoryObj } from "@storybook/react";
import {
	MockProxyLatencies,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceApp,
	MockWorkspaceAppStatus,
} from "testHelpers/entities";
import { WorkspaceAppStatus } from "./WorkspaceAppStatus";
import { getPreferredProxy, ProxyContext } from "contexts/ProxyContext";

const meta: Meta<typeof WorkspaceAppStatus> = {
	title: "modules/workspaces/WorkspaceAppStatus",
	component: WorkspaceAppStatus,
	decorators: [
		(Story) => (
			<ProxyContext.Provider
				value={{
					proxyLatencies: MockProxyLatencies,
					proxy: getPreferredProxy([], undefined),
					proxies: [],
					isLoading: false,
					isFetched: true,
					clearProxy: () => {
						return;
					},
					setProxy: () => {
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
type Story = StoryObj<typeof WorkspaceAppStatus>;

export const Complete: Story = {
	args: {
		status: MockWorkspaceAppStatus,
	},
};

export const Failure: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "failure",
			message: "Couldn't figure out how to start the dev server",
		},
	},
};

export const Working: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			state: "working",
			message: "Starting dev server...",
			uri: "",
		},
	},
};

export const LongURI: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			uri: "https://www.google.com/search?q=hello+world+plus+a+lot+of+other+words",
		},
	},
};

export const FileURI: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			uri: "file:///Users/jason/Desktop/test.txt",
		},
	},
};

export const LongMessage: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			message:
				"This is a long message that will wrap around the component. It should wrap many times because this is very very very very very long.",
		},
	},
};

export const WithApp: Story = {
	args: {
		status: MockWorkspaceAppStatus,
		app: {
			...MockWorkspaceApp,
		},
		agent: MockWorkspaceAgent,
		workspace: MockWorkspace,
	},
};
