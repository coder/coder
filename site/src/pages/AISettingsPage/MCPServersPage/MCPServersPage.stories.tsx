import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { mcpServerConfigKey } from "#/api/queries/chats";
import type * as TypesGen from "#/api/typesGenerated";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
} from "#/testHelpers/storybook";
import AddMCPServerPage from "./AddMCPServerPage/AddMCPServerPage";
import MCPServersPage from "./MCPServersPage";
import { orgSearchParam } from "./organizationParam";
import { MockCoderMCPServer } from "./testFixtures";
import UpdateMCPServerPage from "./UpdateMCPServerPage/UpdateMCPServerPage";

const MockOrganization2MCPServer: TypesGen.MCPServerConfig = {
	...MockCoderMCPServer,
	id: "mcp-org2",
	display_name: "Org2 Search",
	slug: "org2-search",
	organization_id: MockOrganization2.id,
};

const mockOrganization2MCPServerQueryKey = mcpServerConfigKey(
	MockOrganization2.id,
	MockOrganization2MCPServer.id,
);

const RefetchServerDetailProbe: FC = () => {
	const queryClient = useQueryClient();
	return (
		<button
			type="button"
			onClick={() =>
				void queryClient.refetchQueries({
					queryKey: mockOrganization2MCPServerQueryKey,
					exact: true,
				})
			}
		>
			Refetch server detail
		</button>
	);
};

const withRefetchServerDetailProbe: Decorator = (Story) => (
	<>
		<RefetchServerDetailProbe />
		<Story />
	</>
);

const ListRedirectProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return <div>list-org:{searchParams.get(orgSearchParam) ?? "none"}</div>;
};

const DetailRedirectProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return <div>detail-org:{searchParams.get(orgSearchParam) ?? "none"}</div>;
};

const meta = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServersPage",
	component: MCPServersPage,
	decorators: [withAuthProvider, withDashboardProvider],
	parameters: {
		layout: "fullscreen",
		user: MockUserOwner,
		permissions: { editDeploymentConfig: true },
		organizations: [MockDefaultOrganization],
	},
} satisfies Meta<typeof MCPServersPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const ListUsesDefaultOrganization: Story = {
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
			);
		});
		await expect(canvas.getByText("Coder")).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).not.toBeInTheDocument();
	},
};

export const ListSwitchesOrganization: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockOrganization2.id
					? [MockOrganization2MCPServer]
					: [MockCoderMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await expect(await canvas.findByText("Org2 Search")).toBeVisible();
		expect(canvas.queryByText("Coder")).not.toBeInTheDocument();
		expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
			MockOrganization2.id,
		);
	},
};

export const AddUsesDefaultOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				expect.objectContaining({
					display_name: "GitHub",
					slug: "github",
					url: "https://api.githubcopilot.com/mcp/",
				}),
			);
		});
	},
};

export const AddToSelectedOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers/add",
				searchParams: { [orgSearchParam]: MockOrganization2.name },
			},
			routing: [
				{ path: "/ai/settings/mcp-servers/add", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers/:serverId",
					element: <DetailRedirectProbe />,
				},
			],
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", {
				name: `Organization ${MockOrganization2.display_name}`,
			}),
		).toHaveTextContent(MockOrganization2.display_name);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockOrganization2.id,
				expect.objectContaining({ display_name: "GitHub" }),
			);
		});
		await expect(
			await canvas.findByText(`detail-org:${MockOrganization2.name}`),
		).toBeVisible();
	},
};

export const AddDisablesOrganizationWhileSaving: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockImplementation(
			() => new Promise(() => {}),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(
				canvas.getByRole("button", {
					name: `Organization ${MockDefaultOrganization.display_name}`,
				}),
			).toBeDisabled();
		});
	},
};

export const AddSwitchesOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.type(canvas.getByLabelText(/display name/i), "GitHub");
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("GitHub");
		await expect(
			canvas.getByRole("link", { name: /back to mcp servers/i }),
		).toHaveAttribute(
			"href",
			`/ai/settings/mcp-servers?org=${MockOrganization2.name}`,
		);
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await waitFor(() => {
			expect(API.experimental.createMCPServerConfig).toHaveBeenCalledWith(
				MockOrganization2.id,
				expect.objectContaining({ display_name: "GitHub" }),
			);
		});
	},
};

export const UpdateShowsDetailLoadError: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockRejectedValue(
			new Error("Failed to load MCP server."),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText("Failed to load MCP server."),
		).toBeVisible();
		expect(canvas.queryByLabelText(/display name/i)).not.toBeInTheDocument();
	},
};

export const UpdateLoadsServerById: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await waitFor(() => {
			expect(API.experimental.getMCPServerConfig).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				MockCoderMCPServer.id,
			);
		});
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("Coder");
	},
};

export const RowClickCarriesSelectedOrganization: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: [
				{ path: "/ai/settings/mcp-servers", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers/:serverId",
					element: <DetailRedirectProbe />,
				},
			],
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockOrganization2.id
					? [MockOrganization2MCPServer]
					: [MockCoderMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await userEvent.click(await canvas.findByText("Org2 Search"));
		await expect(
			await canvas.findByText(`detail-org:${MockOrganization2.name}`),
		).toBeVisible();
	},
};

const mockNotFoundError = (() => {
	const error = mockApiError({ message: "Resource not found" });
	return { ...error, response: { ...error.response, status: 404 } };
})();

export const UpdateWrongOrganizationRedirectsToSelectedList: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/mcp-servers/${MockOrganization2MCPServer.id}`,
				searchParams: { [orgSearchParam]: MockDefaultOrganization.name },
			},
			routing: [
				{ path: "/ai/settings/mcp-servers/:serverId", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers",
					element: <ListRedirectProbe />,
				},
			],
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockRejectedValue(
			mockNotFoundError,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByText(`list-org:${MockDefaultOrganization.name}`),
		).toBeVisible();
		expect(API.experimental.getMCPServerConfig).toHaveBeenCalledWith(
			MockDefaultOrganization.id,
			MockOrganization2MCPServer.id,
		);
	},
};

export const UpdateCachedNotFoundRedirects: Story = {
	render: () => <UpdateMCPServerPage />,
	decorators: [withRefetchServerDetailProbe],
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: mockOrganization2MCPServerQueryKey,
				data: MockOrganization2MCPServer,
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/mcp-servers/${MockOrganization2MCPServer.id}`,
				searchParams: { [orgSearchParam]: MockOrganization2.name },
			},
			routing: [
				{ path: "/ai/settings/mcp-servers/:serverId", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers",
					element: <ListRedirectProbe />,
				},
			],
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockRejectedValue(
			mockNotFoundError,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue(
			MockOrganization2MCPServer.display_name,
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Refetch server detail" }),
		);
		await expect(
			await canvas.findByText(`list-org:${MockOrganization2.name}`),
		).toBeVisible();
	},
};

export const UpdateRefetchErrorKeepsCachedForm: Story = {
	render: () => <UpdateMCPServerPage />,
	decorators: [withRefetchServerDetailProbe],
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		queries: [
			{
				key: mockOrganization2MCPServerQueryKey,
				data: MockOrganization2MCPServer,
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/mcp-servers/${MockOrganization2MCPServer.id}`,
				searchParams: { [orgSearchParam]: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockRejectedValue(
			mockApiError({ message: "failed to refresh MCP server" }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const displayName = canvas.getByLabelText(/display name/i);
		await userEvent.clear(displayName);
		await userEvent.type(displayName, "Edited name");
		await userEvent.click(
			canvas.getByRole("button", { name: "Refetch server detail" }),
		);
		const alert = await canvas.findByRole("alert");
		await expect(
			within(alert).getByRole("heading", {
				name: /failed to refresh mcp server/i,
			}),
		).toBeVisible();
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue(
			"Edited name",
		);
	},
};
