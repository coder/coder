import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, spyOn, within } from "storybook/test";
import { reactRouterParameters } from "storybook-addon-remix-react-router";
import { API } from "#/api/api";
import type { MCPServerConfigACL } from "#/api/typesGenerated";
import { MockDefaultOrganization, MockGroup } from "#/testHelpers/entities";
import { withDashboardProvider } from "#/testHelpers/storybook";
import { MockCoderMCPServer } from "../testFixtures";
import MCPServerSharingPage from "./MCPServerSharingPage";

const server = {
	...MockCoderMCPServer,
	organization_id: MockDefaultOrganization.id,
};
const acl: MCPServerConfigACL = {
	users: [],
	groups: [{ ...MockGroup, role: "read" }],
};

const meta: Meta<typeof MCPServerSharingPage> = {
	title: "pages/AISettingsPage/MCPServersPage/MCPServerSharingPage",
	component: MCPServerSharingPage,
	decorators: [withDashboardProvider],
	parameters: {
		features: ["template_rbac"],
		reactRouter: reactRouterParameters({
			location: {
				path: `/ai/settings/mcp-servers/${server.id}/sharing`,
				searchParams: { organization: MockDefaultOrganization.name },
			},
			routing: [
				{
					path: "/ai/settings/mcp-servers/:serverId/sharing",
					useStoryElement: true,
				},
			],
		}),
	},
};

export default meta;
type Story = StoryObj<typeof MCPServerSharingPage>;

export const Default: Story = {
	beforeEach: () => {
		spyOn(API.experimental, "getMCPServerConfig").mockResolvedValue(server);
		spyOn(API.experimental, "getMCPServerConfigACL").mockResolvedValue(acl);
		spyOn(API.experimental, "updateMCPServerConfigACL").mockResolvedValue();
		spyOn(API, "checkAuthorization").mockResolvedValue({
			canEdit: true,
			canShare: true,
		});
	},
	play: async ({ canvasElement }) => {
		const canvas = within(canvasElement);
		await expect(
			canvas.findByText(`Share ${server.display_name}`),
		).resolves.toBeVisible();
		await expect(
			canvas.findByText(MockGroup.display_name),
		).resolves.toBeVisible();
		expect(API.experimental.getMCPServerConfig).toHaveBeenCalledWith(server.id);
		expect(API.experimental.getMCPServerConfigACL).toHaveBeenCalledWith(
			server.id,
		);
	},
};
