import type { Decorator, Meta, StoryObj } from "@storybook/react-vite";
import type { FC } from "react";
import { useQueryClient } from "react-query";
import { useSearchParams } from "react-router";
import { expect, spyOn, userEvent, waitFor, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import { mcpServerConfigKey } from "#/api/queries/chats";
import { organizationsPermissions } from "#/api/queries/organizations";
import type * as TypesGen from "#/api/typesGenerated";
import { useDashboard } from "#/modules/dashboard/useDashboard";
import {
	MockDefaultOrganization,
	MockOrganization2,
	MockOrganization3,
	MockUserOwner,
	mockApiError,
} from "#/testHelpers/entities";
import {
	withAuthProvider,
	withDashboardProvider,
	withToaster,
} from "#/testHelpers/storybook";
import AddMCPServerPage from "./AddMCPServerPage/AddMCPServerPage";
import MCPServersPage from "./MCPServersPage";
import { orgSearchParam } from "./organizationParam";
import { MockCoderMCPServer, MockGitHubMCPServer } from "./testFixtures";
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

type MCPOrganizationStoryPermissions = Readonly<{
	view?: boolean;
	create?: boolean;
	update?: boolean;
	delete?: boolean;
}>;

const organizationPermissionsResponse = (
	permissionsByOrganizationId: Readonly<
		Record<string, MCPOrganizationStoryPermissions>
	>,
	checks: Readonly<Record<string, TypesGen.AuthorizationCheck>>,
) =>
	Object.fromEntries(
		Object.keys(checks).map((key) => {
			const separator = key.indexOf(".");
			const organizationId = key.slice(0, separator);
			const permission = key.slice(separator + 1);
			const permissions = permissionsByOrganizationId[organizationId];
			const allowed =
				permission === "viewMCPServerConfigs"
					? permissions?.view
					: permission === "createMCPServerConfig"
						? permissions?.create
						: permission === "updateMCPServerConfig"
							? permissions?.update
							: permission === "deleteMCPServerConfig"
								? permissions?.delete
								: false;
			return [key, Boolean(allowed)];
		}),
	);

const mockOrganizationPermissions = (
	permissionsByOrganizationId: Readonly<
		Record<string, MCPOrganizationStoryPermissions>
	>,
) =>
	spyOn(API, "checkAuthorization").mockImplementation(async ({ checks }) =>
		organizationPermissionsResponse(permissionsByOrganizationId, checks),
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

const RefetchPermissionsProbe: FC = () => {
	const queryClient = useQueryClient();
	const { organizations } = useDashboard();
	return (
		<button
			type="button"
			onClick={() =>
				void queryClient.refetchQueries({
					queryKey: organizationsPermissions(
						organizations.map((organization) => organization.id),
					).queryKey,
					exact: true,
				})
			}
		>
			Refetch permissions
		</button>
	);
};

const withRefetchPermissionsProbe: Decorator = (Story) => (
	<>
		<RefetchPermissionsProbe />
		<Story />
	</>
);

const OrganizationSearchParamProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return (
		<output aria-label="Organization query parameter">
			{searchParams.get(orgSearchParam) ?? "none"}
		</output>
	);
};

const withOrganizationSearchParamProbe: Decorator = (Story) => (
	<>
		<OrganizationSearchParamProbe />
		<Story />
	</>
);

const ListRedirectProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return <div>list-org:{searchParams.get(orgSearchParam) ?? "none"}</div>;
};

const AddRedirectProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return <h1>add-org:{searchParams.get(orgSearchParam) ?? "none"}</h1>;
};

const DetailRedirectProbe: FC = () => {
	const [searchParams] = useSearchParams();
	return <h1>detail-org:{searchParams.get(orgSearchParam) ?? "none"}</h1>;
};

const meta = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServersPage",
	component: MCPServersPage,
	decorators: [withToaster, withAuthProvider, withDashboardProvider],
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

export const OrgAdminCanViewMCPServers: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("cell", { name: /Coder/ }),
		).toBeVisible();
	},
};

export const ReadOnlyOrgAdminCannotModifyMCPServers: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: false,
			updateAnyMCPServerConfig: false,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("cell", { name: /Coder/ }),
		).toBeVisible();
		expect(
			canvas.queryByRole("button", { name: "Add server" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: /Coder/ }),
		).not.toBeInTheDocument();
	},
};

export const DeleteOnlyOrgAdminCanOpenMCPServer: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: true,
		},
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
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { delete: true },
			[MockOrganization2.id]: { delete: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockOrganization2.id
					? [MockOrganization2MCPServer]
					: [MockCoderMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await userEvent.click(
			await canvas.findByRole("button", { name: /Org2 Search/ }),
		);
		await expect(
			await canvas.findByRole("heading", {
				name: `detail-org:${MockOrganization2.name}`,
			}),
		).toBeVisible();
	},
};

export const UpdateOnlyOrgAdminUsesAuthorizedOrganization: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			createAnyMCPServerConfig: false,
			updateAnyMCPServerConfig: true,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers",
				searchParams: { [orgSearchParam]: MockDefaultOrganization.name },
			},
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockOrganization2.id]: { update: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockImplementation(
			async (organization) =>
				organization === MockOrganization2.id
					? [MockOrganization2MCPServer]
					: [MockCoderMCPServer],
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			await canvas.findByRole("cell", { name: /Org2 Search/ }),
		).toBeVisible();
		expect(canvas.queryByText("Coder")).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).not.toBeInTheDocument();
		expect(API.experimental.getMCPServerConfigs).toHaveBeenCalledWith(
			MockOrganization2.id,
		);
	},
};

export const ListCanAddToCreateOnlyOrganization: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: [
				{ path: "/ai/settings/mcp-servers", useStoryElement: true },
				{
					path: "/ai/settings/mcp-servers/add",
					element: <AddRedirectProbe />,
				},
			],
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true },
			[MockOrganization2.id]: { create: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).not.toBeInTheDocument();
		const addButton = canvas.getByRole("button", {
			name: `Add server to ${MockOrganization2.display_name}`,
		});
		await expect(addButton).toHaveAttribute(
			"title",
			`Add server to ${MockOrganization2.display_name}`,
		);
		await userEvent.click(addButton);
		await expect(
			await canvas.findByRole("heading", {
				name: `add-org:${MockOrganization2.name}`,
			}),
		).toBeVisible();
	},
};

export const ListDisambiguatesCollidingCreateOnlyTargets: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [
			MockDefaultOrganization,
			{
				...MockOrganization2,
				name: "create-b",
				display_name: "Create Target",
			},
			{
				...MockOrganization3,
				name: "create-c",
				display_name: "Create Target",
			},
		],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true },
			[MockOrganization2.id]: { create: true },
			[MockOrganization3.id]: { create: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		const addButton = canvas.getByRole("button", {
			name: /^Add server to Create Target \((create-b|create-c)\)$/,
		});
		await expect(addButton).toBeVisible();
	},
};

export const ListDisambiguatesAddTargetFromSelectedOrganization: Story = {
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [
			MockDefaultOrganization,
			{
				...MockOrganization2,
				display_name: MockDefaultOrganization.display_name,
			},
		],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true },
			[MockOrganization2.id]: { create: true },
		});
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		const addButton = canvas.getByRole("button", {
			name: `Add server to ${MockDefaultOrganization.display_name} (${MockOrganization2.name})`,
		});
		await expect(addButton).toBeVisible();
	},
};

export const AddDeepLinkShowsSingleCreatableOrganization: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers/add",
				searchParams: { [orgSearchParam]: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockOrganization2.id]: { create: true },
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const organization = await canvas.findByLabelText(
			`Organization ${MockOrganization2.display_name}`,
		);
		await expect(organization).toBeVisible();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockOrganization2.display_name}`,
			}),
		).not.toBeInTheDocument();
		await expect(canvas.getByLabelText(/display name/i)).toBeVisible();
	},
};

export const ListSearchFiltersServers: Story = {
	parameters: {
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers" },
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([
			MockCoderMCPServer,
			MockGitHubMCPServer,
		]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByText("Coder")).toBeVisible();
		await expect(canvas.getByText("GitHub")).toBeVisible();
		await expect(
			canvas.getByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		).toBeVisible();
		expect(canvas.queryByText("Organization")).not.toBeInTheDocument();

		const search = canvas.getByRole("searchbox", { name: "Search servers" });
		await userEvent.type(search, "github");
		await expect(canvas.getByText("GitHub")).toBeVisible();
		expect(canvas.queryByText("Coder")).not.toBeInTheDocument();

		await userEvent.clear(search);
		await userEvent.type(search, "no-such-server");
		await expect(
			canvas.getByText("No servers match your search"),
		).toBeVisible();

		await userEvent.clear(search);
		await expect(canvas.getByText("Coder")).toBeVisible();
		await expect(canvas.getByText("GitHub")).toBeVisible();
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

export const ListDisambiguatesCollidingOrganizationNames: Story = {
	parameters: {
		organizations: [
			{
				...MockDefaultOrganization,
				name: "org-a",
				display_name: "Dev",
			},
			{
				...MockOrganization2,
				name: "org-b",
				display_name: "Dev",
			},
		],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers",
				searchParams: { [orgSearchParam]: "org-a" },
			},
			routing: { path: "/ai/settings/mcp-servers" },
		}),
	},
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfigs").mockResolvedValue([]);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.getByRole("button", {
				name: /Organization Dev \(org-a\)/,
			}),
		).toBeVisible();
	},
};

export const OrgAdminCanAddMCPServer: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true, create: true },
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(await canvas.findByLabelText(/display name/i)).toBeVisible();
		await expect(
			canvas.getByRole("button", { name: "Add server" }),
		).toBeVisible();
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await expect(
			body.getByRole("option", { name: "OAuth2" }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: "User OIDC identity" }),
		).not.toBeInTheDocument();
	},
};

export const CreateOnlyOrgAdminCanAddMCPServer: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true, create: true },
			[MockOrganization2.id]: { create: true },
		});
		spyOn(API.experimental, "createMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockOrganization2.display_name,
			}),
		);
		await expect(await canvas.findByLabelText(/display name/i)).toBeVisible();
		await expect(
			canvas.queryByRole("link", { name: /back to mcp servers/i }),
		).not.toBeInTheDocument();
		await expect(
			canvas.queryByRole("button", { name: "Cancel" }),
		).not.toBeInTheDocument();
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
			await body.findByText(
				`MCP server "${MockOrganization2MCPServer.display_name}" added.`,
			),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/^slug/i)).toHaveValue("");
		await expect(canvas.getByLabelText(/server url/i)).toHaveValue("");
		await expect(
			canvas.getByRole("button", { name: "Add server" }),
		).toBeDisabled();
	},
};

export const AddDeniedRequestedOrganizationDoesNotFallback: Story = {
	render: () => <AddMCPServerPage />,
	decorators: [withRefetchPermissionsProbe],
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers/add",
				searchParams: { [orgSearchParam]: MockDefaultOrganization.name },
			},
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		const checkAuthorization = mockOrganizationPermissions({
			[MockDefaultOrganization.id]: {},
			[MockOrganization2.id]: { create: true },
		});
		checkAuthorization.mockImplementationOnce(async ({ checks }) =>
			organizationPermissionsResponse(
				{
					[MockDefaultOrganization.id]: { view: true, create: true },
					[MockOrganization2.id]: { create: true },
				},
				checks,
			),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await canvas.findByLabelText(/display name/i),
			"Draft server",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Refetch permissions" }),
		);
		await expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(canvas.queryByLabelText(/display name/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockOrganization2.display_name}`,
			}),
		).not.toBeInTheDocument();
	},
};

export const AddImplicitOrganizationDeniedAfterPermissionsRefetch: Story = {
	render: () => <AddMCPServerPage />,
	decorators: [withRefetchPermissionsProbe, withOrganizationSearchParamProbe],
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			createAnyMCPServerConfig: true,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		const checkAuthorization = mockOrganizationPermissions({
			[MockDefaultOrganization.id]: {},
			[MockOrganization2.id]: { create: true },
		});
		checkAuthorization.mockImplementationOnce(async ({ checks }) =>
			organizationPermissionsResponse(
				{
					[MockDefaultOrganization.id]: { create: true },
					[MockOrganization2.id]: { create: true },
				},
				checks,
			),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await canvas.findByLabelText(/display name/i),
			"Draft server",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: "Refetch permissions" }),
		);
		await expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(canvas.queryByLabelText(/display name/i)).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", {
				name: `Organization ${MockOrganization2.display_name}`,
			}),
		).not.toBeInTheDocument();
		await expect(
			canvas.getByLabelText("Organization query parameter"),
		).toHaveTextContent(MockDefaultOrganization.name);
	},
};

export const AddFailurePreservesEnteredValues: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true, create: true },
		});
		spyOn(API.experimental, "createMCPServerConfig").mockRejectedValue(
			mockApiError({ message: "Invalid client credentials." }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await userEvent.type(
			await canvas.findByLabelText(/display name/i),
			"GitHub",
		);
		await userEvent.type(
			canvas.getByLabelText(/server url/i),
			"https://api.githubcopilot.com/mcp/",
		);
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await userEvent.click(body.getByRole("option", { name: "OAuth2" }));
		await userEvent.type(canvas.getByLabelText(/client id/i), "client-id");
		await userEvent.type(canvas.getByLabelText(/client secret/i), "secret");
		await userEvent.click(canvas.getByRole("button", { name: "Add server" }));
		await expect(
			await body.findByText("Invalid client credentials."),
		).toBeInTheDocument();
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue("GitHub");
		await expect(canvas.getByLabelText(/^slug/i)).toHaveValue("github");
		await expect(canvas.getByLabelText(/server url/i)).toHaveValue(
			"https://api.githubcopilot.com/mcp/",
		);
		await expect(canvas.getByLabelText(/client id/i)).toHaveValue("client-id");
		await expect(canvas.getByLabelText(/client secret/i)).toHaveValue("secret");
	},
};

export const AddPermissionsRefetchErrorKeepsForm: Story = {
	render: () => <AddMCPServerPage />,
	decorators: [withRefetchPermissionsProbe],
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		const checkAuthorization = spyOn(API, "checkAuthorization");
		checkAuthorization.mockImplementationOnce(async ({ checks }) =>
			Object.fromEntries(Object.keys(checks).map((key) => [key, true])),
		);
		checkAuthorization.mockRejectedValue(
			mockApiError({ message: "failed to refresh permissions" }),
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const displayName = await canvas.findByLabelText(/display name/i);
		await userEvent.type(displayName, "Draft server");
		await userEvent.click(
			canvas.getByRole("button", { name: "Refetch permissions" }),
		);
		const alert = await canvas.findByRole("alert");
		await expect(
			within(alert).getByRole("heading", {
				name: /failed to refresh permissions/i,
			}),
		).toBeVisible();
		await expect(canvas.getByLabelText(/display name/i)).toHaveValue(
			"Draft server",
		);
	},
};

export const AddPickerExcludesNonCreatableOrganizations: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
		},
		organizations: [
			MockDefaultOrganization,
			MockOrganization2,
			MockOrganization3,
		],
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/add" },
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true, create: true },
			[MockOrganization2.id]: { view: true },
			[MockOrganization3.id]: { view: true, create: true },
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await userEvent.click(
			await canvas.findByRole("button", {
				name: `Organization ${MockDefaultOrganization.display_name}`,
			}),
		);
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByRole("option", { name: MockOrganization3.display_name }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: MockOrganization2.display_name }),
		).not.toBeInTheDocument();
	},
};

export const AddDeepLinkedNonCreatableOrganizationCanSwitch: Story = {
	render: () => <AddMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			createAnyMCPServerConfig: true,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: "/ai/settings/mcp-servers/add",
				searchParams: { [orgSearchParam]: MockOrganization2.name },
			},
			routing: { path: "/ai/settings/mcp-servers/add" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { view: true, create: true },
			[MockOrganization2.id]: { view: true },
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(await canvas.findByRole("alert")).toHaveTextContent(
			"You cannot add servers to this organization",
		);
		const picker = canvas.getByRole("button", {
			name: `Organization ${MockOrganization2.display_name}`,
		});
		await expect(picker).toBeVisible();
		await userEvent.click(picker);
		await userEvent.click(
			await body.findByRole("option", {
				name: MockDefaultOrganization.display_name,
			}),
		);
		await expect(await canvas.findByLabelText(/display name/i)).toBeVisible();
		expect(canvas.queryByRole("alert")).not.toBeInTheDocument();
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

export const UpdateOnlyOrgAdminCanUpdateMCPServer: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			updateAnyMCPServerConfig: true,
			deleteAnyMCPServerConfig: false,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { update: true },
		});
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(await canvas.findByLabelText(/display name/i)).toHaveValue(
			"Coder",
		);
		await expect(
			canvas.getByRole("button", { name: "Update server" }),
		).toBeEnabled();
		await expect(
			canvas.getByRole("switch", { name: "Server enabled" }),
		).toBeEnabled();
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await userEvent.click(
			canvas.getByRole("combobox", { name: /authentication method/i }),
		);
		await expect(
			body.getByRole("option", { name: "OAuth2" }),
		).toBeInTheDocument();
		expect(
			body.queryByRole("option", { name: "User OIDC identity" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: "Delete" }),
		).not.toBeInTheDocument();
		expect(
			canvas.queryByRole("button", { name: /delete server/i }),
		).not.toBeInTheDocument();
	},
};

export const UpdateDeniedRequestedOrganizationDoesNotFallback: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			updateAnyMCPServerConfig: true,
			deleteAnyMCPServerConfig: false,
		},
		organizations: [MockDefaultOrganization, MockOrganization2],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/mcp-servers/${MockCoderMCPServer.id}`,
				searchParams: { [orgSearchParam]: MockDefaultOrganization.name },
			},
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: {},
			[MockOrganization2.id]: { update: true },
		});
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(
			MockOrganization2MCPServer,
		);
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(
			await body.findByText("You don't have permission to view this page"),
		).toBeInTheDocument();
		expect(canvas.queryByLabelText(/display name/i)).not.toBeInTheDocument();
		expect(API.experimental.getMCPServerConfig).not.toHaveBeenCalled();
	},
};

export const UserOIDCOrgAdminCannotUpdate: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: true,
			updateAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: {
				view: true,
				update: true,
				delete: true,
			},
		});
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue({
			...MockCoderMCPServer,
			auth_type: "user_oidc",
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(await canvas.findByLabelText(/display name/i)).toHaveValue(
			"Coder",
		);
		await expect(
			canvas.getByRole("button", { name: "Update server" }),
		).toBeDisabled();
		const enabledSwitch = canvas.getByRole("switch", {
			name: "Server enabled",
		});
		await expect(enabledSwitch).toBeDisabled();
		await expect(canvas.getByLabelText(/display name/i)).toBeDisabled();
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(
			canvas.getByLabelText(/authentication method/i),
		).toBeDisabled();
		await expect(
			canvas.getByLabelText(/authentication method/i),
		).toHaveTextContent("User OIDC identity");
		await expect(canvas.getByRole("button", { name: "Delete" })).toBeEnabled();
	},
};

export const DeleteOnlyOrgAdminCanDeleteWithoutUpdating: Story = {
	render: () => <UpdateMCPServerPage />,
	parameters: {
		permissions: {
			editDeploymentConfig: false,
			viewAnyMCPServerConfigs: false,
			updateAnyMCPServerConfig: false,
			deleteAnyMCPServerConfig: true,
		},
		reactRouter: reactRouterParameters({
			location: { path: "/ai/settings/mcp-servers/mcp-coder" },
			routing: { path: "/ai/settings/mcp-servers/:serverId" },
		}),
	},
	beforeEach: () => {
		mockOrganizationPermissions({
			[MockDefaultOrganization.id]: { delete: true },
		});
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(
			MockCoderMCPServer,
		);
		spyOn(API.experimental, "deleteMCPServerConfig").mockResolvedValue();
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		const body = within(canvasElement.ownerDocument.body);
		await expect(await canvas.findByLabelText(/display name/i)).toHaveValue(
			"Coder",
		);
		await expect(
			canvas.getByRole("button", { name: "Update server" }),
		).toBeDisabled();
		const enabledSwitch = canvas.getByRole("switch", {
			name: "Server enabled",
		});
		await expect(enabledSwitch).toBeDisabled();
		await expect(canvas.getByLabelText(/^slug/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/display name/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/server url/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/^transport/i)).toBeDisabled();
		await userEvent.click(canvas.getByRole("button", { name: /details/i }));
		await expect(canvas.getByLabelText(/^description/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/^icon/i)).toBeDisabled();
		await userEvent.click(
			canvas.getByRole("button", { name: /authentication/i }),
		);
		await expect(
			canvas.getByLabelText(/authentication method/i),
		).toBeDisabled();
		await expect(canvas.getByLabelText(/client id/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/client secret/i)).toBeDisabled();
		await userEvent.click(canvas.getByRole("button", { name: /behavior/i }));
		await expect(canvas.getByLabelText(/^availability/i)).toBeDisabled();
		await expect(
			canvas.getByRole("switch", { name: "Model intent" }),
		).toBeDisabled();
		await expect(canvas.getByLabelText(/tool allow list/i)).toBeDisabled();
		await expect(canvas.getByLabelText(/tool deny list/i)).toBeDisabled();
		await userEvent.click(canvas.getByRole("button", { name: "Delete" }));
		await userEvent.click(
			await body.findByRole("button", { name: "Delete MCP server" }),
		);
		await waitFor(() => {
			expect(API.experimental.deleteMCPServerConfig).toHaveBeenCalledWith(
				MockDefaultOrganization.id,
				MockCoderMCPServer.id,
			);
		});
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
