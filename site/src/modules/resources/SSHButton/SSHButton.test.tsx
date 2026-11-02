import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { API } from "#/api/api";
import type { Workspace } from "#/api/typesGenerated";
import {
	MockDeploymentSSH,
	MockUserMember,
	MockUserOwner,
	MockWorkspace,
	MockWorkspaceAgent,
	MockWorkspaceAgentDisconnected,
	MockWorkspaceResource,
	MockWorkspaceResourceMultipleAgents,
	MockWorkspaceSubAgent,
} from "#/testHelpers/entities";
import { renderWithAuth } from "#/testHelpers/renderHelpers";
import { AgentSSHButton } from "./SSHButton";

const mockDeploymentSSHWithoutHostnameSuffix = {
	...MockDeploymentSSH,
	hostname_suffix: "",
};

const mockResourceWithSubAgent = {
	...MockWorkspaceResource,
	agents: [MockWorkspaceAgent, MockWorkspaceSubAgent],
};

afterEach(() => {
	vi.restoreAllMocks();
});

describe.each([
	{
		name: "owned single-agent workspace",
		resources: MockWorkspace.latest_build.resources,
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh test-workspace.coder",
		cliCommand: "coder ssh TestUser/test-workspace",
	},
	{
		name: "shared single-agent workspace",
		resources: MockWorkspace.latest_build.resources,
		owner: MockUserMember,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser2.coder",
		cliCommand: "coder ssh TestUser2/test-workspace",
	},
	{
		name: "multiple agents within one resource",
		resources: [MockWorkspaceResourceMultipleAgents],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
	{
		name: "agents across multiple resources",
		resources: [
			MockWorkspaceResource,
			{
				...MockWorkspaceResource,
				id: "second-resource",
				agents: [MockWorkspaceAgentDisconnected],
			},
		],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
	{
		name: "parent agent with a devcontainer subagent",
		resources: [mockResourceWithSubAgent],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
	{
		name: "devcontainer subagent",
		resources: [mockResourceWithSubAgent],
		owner: MockUserOwner,
		agent: MockWorkspaceSubAgent,
		sshCommand: "ssh a-workspace-sub-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-sub-agent",
	},
	{
		name: "zero agents",
		resources: [{ ...MockWorkspaceResource, agents: [] }],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
	{
		name: "unknown agents",
		resources: [{ ...MockWorkspaceResource, agents: undefined }],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
	{
		name: "no resources",
		resources: [],
		owner: MockUserOwner,
		agent: MockWorkspaceAgent,
		sshCommand: "ssh a-workspace-agent.test-workspace.TestUser.coder",
		cliCommand: "coder ssh TestUser/test-workspace.a-workspace-agent",
	},
])("$name", ({ resources, owner, agent, sshCommand, cliCommand }) => {
	it.each([
		{ name: "native SSH", config: MockDeploymentSSH, command: sshCommand },
		{
			name: "CLI fallback",
			config: mockDeploymentSSHWithoutHostnameSuffix,
			command: cliCommand,
		},
	])("copies the $name command", async ({ config, command }) => {
		const user = userEvent.setup();
		const writeText = vi
			.spyOn(navigator.clipboard, "writeText")
			.mockResolvedValue();
		vi.spyOn(API, "getDeploymentSSHConfig").mockResolvedValue(config);
		const mockWorkspace: Workspace = {
			...MockWorkspace,
			owner_id: owner.id,
			owner_name: owner.username,
			latest_build: {
				...MockWorkspace.latest_build,
				workspace_owner_id: owner.id,
				workspace_owner_name: owner.username,
				resources,
			},
		};
		renderWithAuth(
			<AgentSSHButton workspace={mockWorkspace} agentName={agent.name} />,
		);
		await user.click(
			await screen.findByRole("button", { name: "Connect via SSH" }),
		);
		const copyButtons = await screen.findAllByRole("button", {
			name: "Copy code",
		});
		await user.click(copyButtons[copyButtons.length - 1]);
		await waitFor(() => expect(writeText).toHaveBeenCalledWith(command));
	});
});

describe("legacy hostname prefix", () => {
	it.each([
		{
			name: "default prefix collision with a custom suffix",
			workspaceName: "coder",
			hostnamePrefix: "coder.",
			command: "ssh a-workspace-agent.coder.TestUser.dev",
		},
		{
			name: "custom prefix collision",
			workspaceName: "work-lab",
			hostnamePrefix: "work-",
			command: "ssh a-workspace-agent.work-lab.TestUser.dev",
		},
		{
			name: "case-insensitive prefix collision",
			workspaceName: "coder",
			hostnamePrefix: "CoDeR.",
			command: "ssh a-workspace-agent.coder.TestUser.dev",
		},
		{
			name: "case-insensitive hostname collision",
			workspaceName: "CoDeR",
			hostnamePrefix: "coder.",
			command: "ssh a-workspace-agent.CoDeR.TestUser.dev",
		},
		{
			name: "prefix spanning the workspace and suffix",
			workspaceName: "coder",
			hostnamePrefix: "coder.d",
			command: "ssh a-workspace-agent.coder.TestUser.dev",
		},
		{
			name: "noncolliding prefix",
			workspaceName: "mycoder",
			hostnamePrefix: "coder.",
			command: "ssh mycoder.dev",
		},
		{
			name: "disabled prefix",
			workspaceName: "coder",
			hostnamePrefix: "",
			command: "ssh coder.dev",
		},
	])(
		"copies the safe command for $name",
		async ({ workspaceName, hostnamePrefix, command }) => {
			const user = userEvent.setup();
			const writeText = vi
				.spyOn(navigator.clipboard, "writeText")
				.mockResolvedValue();
			const mockDeploymentSSH = {
				...MockDeploymentSSH,
				hostname_prefix: hostnamePrefix,
				hostname_suffix: "dev",
			};
			vi.spyOn(API, "getDeploymentSSHConfig").mockResolvedValue(
				mockDeploymentSSH,
			);
			const mockWorkspace: Workspace = {
				...MockWorkspace,
				name: workspaceName,
				latest_build: {
					...MockWorkspace.latest_build,
					workspace_name: workspaceName,
				},
			};
			renderWithAuth(
				<AgentSSHButton
					workspace={mockWorkspace}
					agentName={MockWorkspaceAgent.name}
				/>,
			);
			await user.click(
				await screen.findByRole("button", { name: "Connect via SSH" }),
			);
			const copyButtons = await screen.findAllByRole("button", {
				name: "Copy code",
			});
			await user.click(copyButtons[copyButtons.length - 1]);
			await waitFor(() => expect(writeText).toHaveBeenCalledWith(command));
		},
	);
});
