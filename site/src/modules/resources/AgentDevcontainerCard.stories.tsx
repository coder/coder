import { chromatic } from "testHelpers/chromatic";
import {
	MockListeningPortsResponse,
	MockPrimaryWorkspaceProxy,
	MockTemplate,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentContainer,
	MockWorkspaceAgentContainerPorts,
	MockWorkspaceAgentDevcontainer,
	MockWorkspaceApp,
	MockWorkspaceProxies,
	MockWorkspaceSubAgent,
	mockApiError,
} from "testHelpers/entities";
import {
	withDashboardProvider,
	withProxyProvider,
} from "testHelpers/storybook";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { API } from "api/api";
import { getPreferredProxy } from "contexts/ProxyContext";
import { spyOn, userEvent, within } from "storybook/test";
import { AgentDevcontainerCard } from "./AgentDevcontainerCard";

const meta: Meta<typeof AgentDevcontainerCard> = {
	title: "modules/resources/AgentDevcontainerCard",
	component: AgentDevcontainerCard,
	args: {
		devcontainer: MockWorkspaceAgentDevcontainer,
		workspace: MockWorkspace,
		wildcardHostname: "*.wildcard.hostname",
		parentAgent: MockWorkspaceAgent,
		template: MockTemplate,
		subAgents: [MockWorkspaceSubAgent],
	},
	decorators: [withProxyProvider(), withDashboardProvider],
	parameters: {
		chromatic,
		queries: [
			{
				key: ["portForward", MockWorkspaceSubAgent.id],
				data: MockListeningPortsResponse,
			},
		],
	},
};

export default meta;
type Story = StoryObj<typeof AgentDevcontainerCard>;

export const HasError: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			error: "unable to inject devcontainer with agent",
			agent: undefined,
		},
	},
};

export const NoPorts: Story = {};

export const WithPorts: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
		},
	},
};

export const Dirty: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
		},
	},
};

export const Recreating: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			dirty: true,
			status: "starting",
			container: undefined,
		},
		subAgents: [],
	},
};

export const Stopping: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			status: "stopping",
		},
		subAgents: [],
	},
};

export const Deleting: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			status: "deleting",
		},
		subAgents: [],
	},
};

export const NoContainerOrSubAgent: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: undefined,
			agent: undefined,
		},
		subAgents: [],
	},
};

export const NoContainerOrAgentOrName: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: undefined,
			agent: undefined,
			name: "",
		},
		subAgents: [],
	},
};

export const NoSubAgent: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			agent: undefined,
		},
		subAgents: [],
	},
};

export const SubAgentConnecting: Story = {
	args: {
		subAgents: [
			{
				...MockWorkspaceSubAgent,
				status: "connecting",
			},
		],
	},
};

export const WithAppsAndPorts: Story = {
	args: {
		devcontainer: {
			...MockWorkspaceAgentDevcontainer,
			container: {
				...MockWorkspaceAgentContainer,
				ports: MockWorkspaceAgentContainerPorts,
			},
		},
		subAgents: [
			{
				...MockWorkspaceSubAgent,
				apps: [MockWorkspaceApp],
			},
		],
	},
};

export const WithPortForwarding: Story = {
	decorators: [
		withProxyProvider({
			proxy: getPreferredProxy(MockWorkspaceProxies, MockPrimaryWorkspaceProxy),
			proxies: MockWorkspaceProxies,
		}),
	],
};

export const WithDeleteError: Story = {
	beforeEach: () => {
		spyOn(API, "deleteDevContainer").mockRejectedValue(
			mockApiError({
				message: "An error occurred stopping the container",
				detail:
					"stop ba5eb2bc1cc415a57552f7f1fd369ad13cbebe70030d46aa9b3b9253b383a81c: exit status 1: Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?: exit status 1",
			}),
		);
	},
	play: async ({ canvasElement }) => {
		const user = userEvent.setup();
		const body = canvasElement.ownerDocument.body;
		const canvas = within(canvasElement);

		const moreActionsButton = canvas.getByRole("button", {
			name: "Dev Container actions",
		});
		await user.click(moreActionsButton);

		const deleteButton = await within(body).findByText("Delete…");
		await user.click(deleteButton);

		const confirmDeleteButton = within(body).getByRole("button", {
			name: "Delete",
		});
		await user.click(confirmDeleteButton);
	},
};
