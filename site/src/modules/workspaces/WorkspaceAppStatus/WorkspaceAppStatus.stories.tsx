import type { Meta, StoryObj } from "@storybook/react";
import { ProxyContext, getPreferredProxy } from "contexts/ProxyContext";
import {
	MockProxyLatencies,
	MockWorkspaceAppStatus,
} from "testHelpers/entities";
import { WorkspaceAppStatus } from "./WorkspaceAppStatus";

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

export const LongMessage: Story = {
	args: {
		status: {
			...MockWorkspaceAppStatus,
			message:
				"This is a long message that will wrap around the component. It should wrap many times because this is very very very very very long.",
		},
	},
};
